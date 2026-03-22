# VPS Plan Management - Gap Analysis & Fixes

## UI GAPS (Critical) - ALL DONE

- [x] Create Plan button is DISABLED - `webui/admin/app/plans/page.tsx:164`
- [x] Edit only allows limit fields - `PlanEditDialog.tsx` only edits `snapshot_limit`, `backup_limit`, `iso_upload_limit`
- [x] Cannot edit core plan properties (name, slug, vcpu, memory_mb, disk_gb, price_monthly, etc.)
- [x] No "Create Plan" dialog component exists - need `PlanCreateDialog.tsx`

## API MISMATCHES (Bugs) - ALL DONE

- [x] API uses PUT, frontend uses PATCH for plan updates - mismatch between backend and frontend
- [x] Verify/fix provisioning plan endpoint for WHMCS - `GET /provisioning/plans/{id}`

## WHMCS INTEGRATION GAPS - ALL DONE

- [x] Plan ID is manual text field - no dropdown/validation in WHMCS module
- [x] No plan sync mechanism - WHMCS cannot list available plans from VirtueStack
- [ ] ~~ChangePackage only supports upgrades - disk shrinking not supported~~ (WONTFIX - technical limitation)

## MISSING TESTS - ALL DONE

- [x] Add `internal/controller/api/admin/plans_test.go` - no tests exist
- [x] Add tests for `internal/controller/services/plan_service.go`
- [x] Add tests for `internal/controller/repository/plan_repo.go`

## SECURITY / DATA INTEGRITY - ALL DONE

- [x] No plan usage count endpoint - cannot see VM count before deletion
- [x] Slug validation too restrictive - `alphanum` prevents hyphens like `standard-1`
- [x] Missing authorization granularity - all admins can manage plans (see detailed steps below)

## SCHEMA CLEANUP - ALL DONE

- [x] Clean up unused `max_*` columns in plans table or add to Go model

---

# Authorization Granularity Implementation - DONE

All steps completed:
- [x] Permission model defined in `internal/controller/models/permission.go`
- [x] Migration added `permissions JSONB` column to `admins` table
- [x] Permission middleware created in `internal/controller/api/middleware/permissions.go`
- [x] Routes updated with permission middleware
- [x] Permission management API endpoints added
- [x] Frontend permission checks and UI support

---

# Admin Portal UI Gaps - ALL DONE

### Nodes Page
- [x] NodeCreateDialog.tsx created
- [x] NodeEditDialog.tsx created
- [x] "Add Node" button enabled

### Customers Page
- [x] CustomerCreateDialog.tsx created
- [x] CustomerEditDialog.tsx created
- [x] "Add Customer" button enabled

### IP Sets Page
- [x] IPSetEditDialog.tsx created
- [x] IPSetDetailDialog.tsx created
- [x] "View" and "Edit" buttons enabled

### Templates Page
- [x] TemplateEditDialog.tsx created

### VMs Page
- [x] VMCreateDialog.tsx created
- [x] VMEditDialog.tsx created

---

# Customer Portal - NO ACTION NEEDED

- [x] "Provisioning..." button disabled - INTENTIONAL (shows state)
- [x] Spinning icon for provisioning VMs - INTENTIONAL (shows state)

---

## Summary

All todo items from the original gap analysis have been completed. The admin portal now has full CRUD functionality for:
- Plans
- Nodes
- Customers
- IP Sets
- Templates
- VMs

Fine-grained authorization has been implemented with a permissions system.