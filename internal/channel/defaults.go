package channel

import "time"

// The quiet window a connection starts with and the most it may wait.
const (
	DefaultQuietWindow = 3 * time.Second
	DefaultQuietMax    = 10 * time.Second
	MaxQuiet           = 10 * time.Second
)

// The set replies a connection starts with; an operator may change them.
const (
	DefaultReplyFailure         = "Sorry, something went wrong on my side. Please try again in a moment."
	DefaultReplyVoice           = "Sorry, I can't listen to voice messages yet. Please type your message."
	DefaultReplyPhotoUnreadable = "Sorry, I couldn't read that photo. Please send it again, or describe it in text."
	DefaultApprovalPrompt       = "{preview}\n\nReply YES or NO"
	DefaultApprovalExpired      = "This request expired. Please ask again."
)
