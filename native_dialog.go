package main

// Wails beta.11's Windows file-dialog adapter returns its internal
// cfd.ErrorCancelled rather than an empty successful selection. That sentinel
// is not exported. Normalize its exact message at the native adapter boundary;
// preserve real dialog errors and never write anything after cancellation.
func normalizeNativeSaveResult(path string, err error) (string, error) {
	if path == "" && err != nil && err.Error() == "cancelled by user" {
		return "", nil
	}
	return path, err
}
