package model

import "time"

// TerminalTemplateMethods are the HTTP methods the ported CHECK constraint
// allows. Validating here turns a bad method into a 400 instead of a 500 from the
// constraint.
var TerminalTemplateMethods = []string{"GET", "POST", "PATCH", "PUT", "DELETE"}

// IsTerminalTemplateMethod reports whether m is allowed.
func IsTerminalTemplateMethod(m string) bool {
	for _, v := range TerminalTemplateMethods {
		if v == m {
			return true
		}
	}
	return false
}

// TerminalTemplate is a saved RouterOS REST call, ported from Mikromagic.
//
// Every migrated row has is_builtin = true — the templates are the product's
// shipped library (two copies: the platform business and nish's), not customer
// data. Built-ins are therefore read-only through the API: a tenant may browse
// them and add their own, but cannot rewrite or delete shipped content.
type TerminalTemplate struct {
	ID          string    `json:"id"`
	BusinessID  string    `json:"business_id"`
	CreatedBy   *string   `json:"created_by,omitempty"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	Category    string    `json:"category"`
	Method      string    `json:"method"`
	Path        string    `json:"path"`
	Body        *string   `json:"body,omitempty"`
	IsBuiltin   bool      `json:"is_builtin"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TerminalTemplateRequest is the create/update body. is_builtin is absent on
// purpose: it is server-owned, so a client cannot mint itself a built-in (or
// clear the flag to make shipped content deletable).
type TerminalTemplateRequest struct {
	Name        string  `json:"name"        binding:"required,min=1,max=120"`
	Description *string `json:"description" binding:"omitempty,max=500"`
	Category    string  `json:"category"    binding:"required,min=1,max=40"`
	Method      string  `json:"method"      binding:"required,min=1,max=10"`
	Path        string  `json:"path"        binding:"required,min=1,max=300"`
	Body        *string `json:"body"        binding:"omitempty,max=20000"`
}
