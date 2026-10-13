// Package views renders the member notice emails.
package views

import "fmt"

// RemovedSubject is the subject of the removal notice.
func RemovedSubject(org string) string { return "You were removed from " + org }

// RemovedText is the plain-text removal notice.
func RemovedText(org string) string {
	return fmt.Sprintf("You are no longer a member of %s.\n\n"+
		"Your sessions have ended and your personal API keys for this organization were revoked.\n\n"+
		"If you think this is a mistake, ask an owner or admin of %s to add you back.\n", org, org)
}

// RoleChangedSubject is the subject of the role change notice.
func RoleChangedSubject(org, role string) string {
	return fmt.Sprintf("Your role in %s is now %s", org, role)
}

// RoleChangedText is the plain-text role change notice.
func RoleChangedText(org, role, consoleURL string) string {
	return fmt.Sprintf("Your role in %s is now %s.\n\n"+
		"Please sign in again to continue with your new permissions.\n\n"+
		"Open the console: %s\n", org, role, consoleURL)
}
