package views

import "fmt"

// ResetSubject is the subject of the password reset email.
func ResetSubject() string { return "Reset your oagent password" }

// ResetText is the plain-text password reset email.
func ResetText(link, expires string) string {
	return fmt.Sprintf("Someone asked to reset the password of your oagent account.\n\n"+
		"Open this link to choose a new password:\n%s\n\n"+
		"The link works once, until %s. If you did not ask for this, you can ignore this email; your password stays the same.\n",
		link, expires)
}

// ChangedSubject is the subject of the "password changed" email.
func ChangedSubject() string { return "Your oagent password was changed" }

// ChangedText is the plain-text "password changed" email.
func ChangedText(signIn string) string {
	return fmt.Sprintf("The password of your oagent account was changed, and you were signed out of your other sessions.\n\n"+
		"Sign in: %s\n\n"+
		"If you did not do this, reset your password now and tell your administrator.\n", signIn)
}
