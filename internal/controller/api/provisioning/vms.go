package provisioning

import (
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"slices"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

// CreateVM handles POST /vms - creates a new VM asynchronously.
// This endpoint is called by WHMCS when a new service is provisioned.
func (h *ProvisioningHandler) CreateVM(c *gin.Context) {
	var req ProvisioningCreateVMRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	idempotencyKey := c.GetHeader("Idempotency-Key")
	password := generateRandomPassword()
	vmReq := buildVMCreateRequest(&req, password, idempotencyKey)

	vm, taskID, err := h.vmService.CreateVM(c.Request.Context(), vmReq, req.CustomerID)
	if err != nil {
		h.logger.Error("failed to create VM", "customer_id", req.CustomerID, "hostname", req.Hostname, "error", err, "correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "VM_CREATE_FAILED", "Internal server error")
		return
	}

	h.logger.Info("VM creation initiated via provisioning API", "vm_id", vm.ID, "task_id", taskID, "customer_id", req.CustomerID, "whmcs_service_id", req.WHMCSServiceID, "correlation_id", middleware.GetCorrelationID(c))
	c.JSON(http.StatusAccepted, models.Response{Data: CreateVMResponse{TaskID: taskID, VMID: vm.ID}})
}

// DeleteVM handles DELETE /vms/:id - terminates a VM asynchronously.
// This endpoint is called by WHMCS when a service is cancelled/terminated.
func (h *ProvisioningHandler) DeleteVM(c *gin.Context) {
	vmID := c.Param("id")

	vm, err := getValidVMWithChecks(c.Request.Context(), h.vmRepo, vmID, h.logger, checkVMOpts{
		AlreadyDeleted: true,
		DeletedErrMsg:   "VM has already been terminated",
	})
	if err != nil {
		respondWithValidationError(c, err)
		return
	}

	taskID, err := h.vmService.DeleteVM(c.Request.Context(), vmID, vm.CustomerID, true)
	if err != nil {
		h.logger.Error("failed to delete VM", "vm_id", vmID, "error", err, "correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "VM_DELETE_FAILED", "Internal server error")
		return
	}

	h.logger.Info("VM termination initiated via provisioning API", "vm_id", vmID, "task_id", taskID, "customer_id", vm.CustomerID, "correlation_id", middleware.GetCorrelationID(c))
	c.JSON(http.StatusAccepted, models.Response{Data: TaskResponse{TaskID: taskID}})
}

// buildVMCreateRequest builds a VMCreateRequest from a ProvisioningCreateVMRequest.
func buildVMCreateRequest(req *ProvisioningCreateVMRequest, password, idempotencyKey string) *models.VMCreateRequest {
	vmReq := &models.VMCreateRequest{
		CustomerID:     req.CustomerID,
		PlanID:         req.PlanID,
		TemplateID:     req.TemplateID,
		Hostname:       req.Hostname,
		Password:       password,
		SSHKeys:        req.SSHKeys,
		WHMCSServiceID: &req.WHMCSServiceID,
		IdempotencyKey: idempotencyKey,
	}
	if req.LocationID != "" {
		vmReq.LocationID = &req.LocationID
	}
	return vmReq
}

// generateRandomPassword creates a cryptographically secure random password
// that always satisfies strength requirements: at least 1 uppercase letter,
// 1 lowercase letter, 1 digit, 1 special character, and 12+ characters total.
func generateRandomPassword() string {
	const (
		upperChars   = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		lowerChars   = "abcdefghijklmnopqrstuvwxyz"
		digitChars   = "0123456789"
		specialChars = "!@#$%^&*"
		allChars     = upperChars + lowerChars + digitChars + specialChars
		totalLen     = 16
	)

	randChar := func(charset string) byte {
		n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			// SECURITY: crypto/rand failure indicates system entropy exhaustion or
			// serious system issue. Fail immediately rather than generating predictable
			// passwords with math/rand (CWE-1241).
			panic(fmt.Sprintf("crypto/rand failure during password generation: %v", err))
		}
		return charset[n.Int64()]
	}

	// Guarantee at least one character from each required class
	required := []byte{
		randChar(upperChars),
		randChar(lowerChars),
		randChar(digitChars),
		randChar(specialChars),
	}

	// Fill remaining characters from the full set
	rest := make([]byte, totalLen-len(required))
	for i := range rest {
		rest[i] = randChar(allChars)
	}

	combined := slices.Concat(required, rest)

	// Shuffle using crypto/rand to avoid predictable placement of required chars
	for i := len(combined) - 1; i > 0; i-- {
		jBig, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			// SECURITY: Consistent with randChar - panic on crypto/rand failure.
			panic(fmt.Sprintf("crypto/rand failure during password shuffle: %v", err))
		}
		j := int(jBig.Int64())
		combined[i], combined[j] = combined[j], combined[i]
	}

	return string(combined)
}
