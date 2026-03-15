// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"time"
)

// RDNSService manages reverse DNS (PTR) records via direct PowerDNS MySQL integration.
// It handles PTR record creation, updates, and deletion for IPv4 and IPv6 addresses.
type RDNSService struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewRDNSService creates a new RDNSService with the given PowerDNS MySQL database connection.
// The db parameter should be a connection to the PowerDNS database (not the VirtueStack database).
func NewRDNSService(db *sql.DB, logger *slog.Logger) *RDNSService {
	return &RDNSService{
		db:     db,
		logger: logger.With("component", "rdns-service"),
	}
}

// SetReverseDNS creates or updates a PTR record for the given IP address.
// It parses the IP address, generates the appropriate in-addr.arpa or ip6.arpa name,
// and upserts the PTR record in the PowerDNS database.
// The SOA serial is automatically incremented for the zone.
func (s *RDNSService) SetReverseDNS(ctx context.Context, ipAddress, hostname string) error {
	// Validate and parse the IP address
	addr, err := netip.ParseAddr(ipAddress)
	if err != nil {
		return fmt.Errorf("invalid IP address %q: %w", ipAddress, err)
	}

	// Generate the reverse DNS name (PTR name)
	ptrName, zoneName := generatePTRName(addr)
	if ptrName == "" {
		return fmt.Errorf("failed to generate PTR name for IP %s", ipAddress)
	}

	// Get the domain/zone ID from PowerDNS
	zoneID, err := s.getZoneID(ctx, zoneName)
	if err != nil {
		return fmt.Errorf("getting zone ID for %s: %w", zoneName, err)
	}

	// Upsert the PTR record
	if err := s.upsertPTRRecord(ctx, zoneID, ptrName, hostname); err != nil {
		return fmt.Errorf("upserting PTR record for %s: %w", ptrName, err)
	}

	// Update SOA serial
	if err := s.updateSOASerial(ctx, zoneID); err != nil {
		s.logger.Warn("failed to update SOA serial", "zone", zoneName, "error", err)
		// Don't fail the operation - PowerDNS may auto-increment serial
	}

	s.logger.Info("PTR record set",
		"ip", ipAddress,
		"ptr_name", ptrName,
		"hostname", hostname,
		"zone", zoneName)

	return nil
}

// DeleteReverseDNS removes a PTR record for the given IP address.
// It parses the IP address and deletes the corresponding PTR record from PowerDNS.
func (s *RDNSService) DeleteReverseDNS(ctx context.Context, ipAddress string) error {
	// Validate and parse the IP address
	addr, err := netip.ParseAddr(ipAddress)
	if err != nil {
		return fmt.Errorf("invalid IP address %q: %w", ipAddress, err)
	}

	// Generate the reverse DNS name (PTR name)
	ptrName, zoneName := generatePTRName(addr)
	if ptrName == "" {
		return fmt.Errorf("failed to generate PTR name for IP %s", ipAddress)
	}

	// Get the domain/zone ID from PowerDNS
	zoneID, err := s.getZoneID(ctx, zoneName)
	if err != nil {
		return fmt.Errorf("getting zone ID for %s: %w", zoneName, err)
	}

	// Delete the PTR record
	if err := s.deletePTRRecord(ctx, zoneID, ptrName); err != nil {
		return fmt.Errorf("deleting PTR record for %s: %w", ptrName, err)
	}

	// Update SOA serial
	if err := s.updateSOASerial(ctx, zoneID); err != nil {
		s.logger.Warn("failed to update SOA serial", "zone", zoneName, "error", err)
	}

	s.logger.Info("PTR record deleted",
		"ip", ipAddress,
		"ptr_name", ptrName,
		"zone", zoneName)

	return nil
}

func (s *RDNSService) GetReverseDNS(ctx context.Context, ipAddress string) (string, error) {
	addr, err := netip.ParseAddr(ipAddress)
	if err != nil {
		return "", fmt.Errorf("invalid IP address %q: %w", ipAddress, err)
	}

	ptrName, zoneName := generatePTRName(addr)
	if ptrName == "" {
		return "", fmt.Errorf("failed to generate PTR name for IP %s", ipAddress)
	}

	zoneID, err := s.getZoneID(ctx, zoneName)
	if err != nil {
		return "", fmt.Errorf("getting zone ID for %s: %w", zoneName, err)
	}

	const query = `SELECT content FROM records WHERE domain_id = ? AND name = ? AND type = 'PTR' LIMIT 1`
	var content string
	err = s.db.QueryRowContext(ctx, query, zoneID, ptrName).Scan(&content)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("querying PTR record: %w", err)
	}

	return content, nil
}

func (s *RDNSService) IsConnected() bool {
	if s.db == nil {
		return false
	}
	return s.db.PingContext(context.Background()) == nil
}

// getZoneID retrieves the zone ID from PowerDNS domains table.
func (s *RDNSService) getZoneID(ctx context.Context, zoneName string) (int64, error) {
	const query = `SELECT id FROM domains WHERE name = ?`

	var zoneID int64
	err := s.db.QueryRowContext(ctx, query, zoneName).Scan(&zoneID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("zone %s not found in PowerDNS", zoneName)
		}
		return 0, fmt.Errorf("querying zone ID: %w", err)
	}

	return zoneID, nil
}

// upsertPTRRecord creates or updates a PTR record in the PowerDNS records table.
// It first tries to update an existing record, and if none exists, inserts a new one.
func (s *RDNSService) upsertPTRRecord(ctx context.Context, zoneID int64, ptrName, hostname string) error {
	// First, try to update existing record
	const updateQuery = `
		UPDATE records 
		SET content = ?, change_date = ?
		WHERE domain_id = ? AND name = ? AND type = 'PTR'`

	changeDate := time.Now().Unix()

	result, err := s.db.ExecContext(ctx, updateQuery, hostname, changeDate, zoneID, ptrName)
	if err != nil {
		return fmt.Errorf("updating PTR record: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}

	// If no rows were updated, insert a new record
	if rowsAffected == 0 {
		const insertQuery = `
			INSERT INTO records (domain_id, name, type, content, ttl, change_date)
			VALUES (?, ?, 'PTR', ?, 3600, ?)`

		_, err := s.db.ExecContext(ctx, insertQuery, zoneID, ptrName, hostname, changeDate)
		if err != nil {
			return fmt.Errorf("inserting PTR record: %w", err)
		}
	}

	return nil
}

// deletePTRRecord removes a PTR record from the PowerDNS records table.
func (s *RDNSService) deletePTRRecord(ctx context.Context, zoneID int64, ptrName string) error {
	const query = `DELETE FROM records WHERE domain_id = ? AND name = ? AND type = 'PTR'`

	result, err := s.db.ExecContext(ctx, query, zoneID, ptrName)
	if err != nil {
		return fmt.Errorf("deleting PTR record: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}

	if rowsAffected == 0 {
		s.logger.Debug("no PTR record found to delete", "ptr_name", ptrName)
	}

	return nil
}

// updateSOASerial increments the SOA serial number for the zone.
// PowerDNS expects the serial to be updated when records change.
func (s *RDNSService) updateSOASerial(ctx context.Context, zoneID int64) error {
	// Get current SOA record
	const selectQuery = `
		SELECT id, content 
		FROM records 
		WHERE domain_id = ? AND type = 'SOA'`

	var recordID int64
	var content string
	err := s.db.QueryRowContext(ctx, selectQuery, zoneID).Scan(&recordID, &content)
	if err != nil {
		if err == sql.ErrNoRows {
			// No SOA record found - this is unusual but not fatal
			return fmt.Errorf("no SOA record found for zone")
		}
		return fmt.Errorf("querying SOA record: %w", err)
	}

	// Parse SOA content: primary_ns responsible_person serial refresh retry expire default_ttl
	parts := strings.Split(content, " ")
	if len(parts) < 3 {
		return fmt.Errorf("malformed SOA record content: %s", content)
	}

	// Parse current serial
	var currentSerial int64
	if _, err := fmt.Sscanf(parts[2], "%d", &currentSerial); err != nil {
		return fmt.Errorf("parsing SOA serial: %w", err)
	}

	// Generate new serial in YYYYMMDDNN format
	newSerial := generateSOASerial(currentSerial)

	// Update the serial in the content
	parts[2] = fmt.Sprintf("%d", newSerial)
	newContent := strings.Join(parts, " ")

	// Update the SOA record
	const updateQuery = `
		UPDATE records 
		SET content = ?, change_date = ?
		WHERE id = ?`

	changeDate := time.Now().Unix()

	_, err = s.db.ExecContext(ctx, updateQuery, newContent, changeDate, recordID)
	if err != nil {
		return fmt.Errorf("updating SOA serial: %w", err)
	}

	return nil
}

// generatePTRName creates the PTR name and zone name from an IP address.
// For IPv4: 192.0.2.1 -> "1.2.0.192.in-addr.arpa", "2.0.192.in-addr.arpa" (or appropriate zone)
// For IPv6: 2001:db8::1 -> "1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa"
func generatePTRName(addr netip.Addr) (ptrName, zoneName string) {
	if addr.Is4() {
		return generateIPv4PTRName(addr)
	}
	return generateIPv6PTRName(addr)
}

// generateIPv4PTRName generates the PTR name for an IPv4 address.
// The PTR name is the reversed IP with .in-addr.arpa suffix.
// The zone name depends on the zone delegation (typically /24 or larger).
func generateIPv4PTRName(addr netip.Addr) (ptrName, zoneName string) {
	// Get the 4-byte representation
	bytes := addr.As4()

	// Build the reversed address
	// e.g., 192.0.2.1 -> 1.2.0.192.in-addr.arpa
	ptrName = fmt.Sprintf("%d.%d.%d.%d.in-addr.arpa", bytes[3], bytes[2], bytes[1], bytes[0])

	// The zone name is typically the /24 or larger delegation
	// For simplicity, we assume a /24 zone (most common for rDNS)
	// e.g., 192.0.2.1 -> 2.0.192.in-addr.arpa
	zoneName = fmt.Sprintf("%d.%d.%d.in-addr.arpa", bytes[2], bytes[1], bytes[0])

	return ptrName, zoneName
}

// generateIPv6PTRName generates the PTR name for an IPv6 address.
// The PTR name is the reversed nibbles with .ip6.arpa suffix.
func generateIPv6PTRName(addr netip.Addr) (ptrName, zoneName string) {
	// Get the 16-byte representation
	bytes := addr.As16()

	// Build the reversed nibbles string
	// Each byte becomes two hex digits, reversed
	var nibbles []string
	for i := 15; i >= 0; i-- {
		nibbles = append(nibbles,
			fmt.Sprintf("%x", bytes[i]&0x0f),
			fmt.Sprintf("%x", (bytes[i]>>4)&0x0f),
		)
	}

	ptrName = strings.Join(nibbles, ".") + ".ip6.arpa"
	zoneName = generateIPv6ReverseZone(bytes, 48)

	return ptrName, zoneName
}

func generateIPv6ReverseZone(bytes [16]byte, prefixBits int) string {
	if prefixBits <= 0 {
		return "ip6.arpa"
	}

	nibbleCount := prefixBits / 4
	if nibbleCount <= 0 {
		return "ip6.arpa"
	}
	if nibbleCount > 32 {
		nibbleCount = 32
	}

	full := make([]byte, 0, 32)
	for i := 0; i < len(bytes); i++ {
		full = append(full,
			"0123456789abcdef"[(bytes[i]>>4)&0x0f],
			"0123456789abcdef"[bytes[i]&0x0f],
		)
	}

	reversed := make([]string, 0, nibbleCount)
	for i := nibbleCount - 1; i >= 0; i-- {
		reversed = append(reversed, string(full[i]))
	}

	if len(reversed) == 0 {
		return "ip6.arpa"
	}

	return strings.Join(reversed, ".") + ".ip6.arpa"
}

// generateSOASerial generates a new SOA serial number in YYYYMMDDNN format.
// If the current serial is already in today's format and less than the max,
// it increments the NN portion. Otherwise, it starts with 01.
func generateSOASerial(currentSerial int64) int64 {
	now := time.Now().UTC()
	todayBase := int64(now.Year()*1000000 + int(now.Month())*10000 + now.Day()*100)

	// If current serial is from today and less than max (99), increment
	if currentSerial >= todayBase && currentSerial < todayBase+99 {
		return currentSerial + 1
	}

	// Otherwise, start with today's date + 01
	return todayBase + 1
}
