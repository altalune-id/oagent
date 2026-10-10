// Package install performs the first-boot setup: the first owner, the system org and its default project.
package install

// Request is the form a person submits to install the instance.
type Request struct {
	Name     string
	Email    string
	Password string
	OrgName  string
}
