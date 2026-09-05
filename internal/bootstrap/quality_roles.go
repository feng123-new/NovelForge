package bootstrap

func init() {
	// Librarian is an existing Phase 5 service; accepting its role override
	// does not create a new agent loop or an Autopilot worker.
	knownRoles["librarian"] = true
}
