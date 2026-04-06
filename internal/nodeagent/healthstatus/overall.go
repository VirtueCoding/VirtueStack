// Package healthstatus contains pure helpers for node-level health decisions.
package healthstatus

// OverallNodeHealthy reports the top-level node health bit from dependency health.
func OverallNodeHealthy(libvirtConnected, storageConnected bool) bool {
	return libvirtConnected && storageConnected
}
