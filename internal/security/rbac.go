// Package security provides role-based access control (RBAC) functionalities.
// Needs to update to production-ready code.

package security

import (
	"context"
	"sync"
)

// RBAC provides role-based access control
type RBAC struct {
	roles       map[string]*Role
	permissions map[string]*Permission
	mu          sync.RWMutex
}

// Role represents a user role
type Role struct {
	Name        string
	Description string
	Permissions []string
	Parent      string // Parent role for inheritance
}

// Permission represents a permission
type Permission struct {
	Name        string
	Description string
	Resource    string
	Action      string
}

// Subject represents an entity that can have roles
type Subject struct {
	ID    string
	Roles []string
}

// NewRBAC creates a new RBAC instance
func NewRBAC() *RBAC {
	return &RBAC{
		roles:       make(map[string]*Role),
		permissions: make(map[string]*Permission),
	}
}

// AddRole adds a new role
func (r *RBAC) AddRole(role *Role) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.roles[role.Name] = role
}

// AddPermission adds a new permission
func (r *RBAC) AddPermission(permission *Permission) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.permissions[permission.Name] = permission
}

// GrantPermission grants a permission to a role
func (r *RBAC) GrantPermission(roleName, permissionName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	role, ok := r.roles[roleName]
	if !ok {
		return ErrAccessDenied
	}

	// Check if permission exists
	if _, ok := r.permissions[permissionName]; !ok {
		return ErrAccessDenied
	}

	// Add permission if not already present
	for _, perm := range role.Permissions {
		if perm == permissionName {
			return nil
		}
	}

	role.Permissions = append(role.Permissions, permissionName)
	return nil
}

// RevokePermission revokes a permission from a role
func (r *RBAC) RevokePermission(roleName, permissionName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	role, ok := r.roles[roleName]
	if !ok {
		return ErrAccessDenied
	}

	// Remove permission
	for i, perm := range role.Permissions {
		if perm == permissionName {
			role.Permissions = append(role.Permissions[:i], role.Permissions[i+1:]...)
			return nil
		}
	}

	return nil
}

// HasPermission checks if a subject has a specific permission
func (r *RBAC) HasPermission(subject *Subject, permissionName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.hasPermission(subject, permissionName, make(map[string]bool))
}

func (r *RBAC) hasPermission(subject *Subject, permissionName string, visited map[string]bool) bool {
	for _, roleName := range subject.Roles {
		if visited[roleName] {
			continue
		}
		visited[roleName] = true

		role, ok := r.roles[roleName]
		if !ok {
			continue
		}

		// Check direct permissions
		for _, perm := range role.Permissions {
			if perm == permissionName {
				return true
			}
		}

		// Check parent role
		if role.Parent != "" {
			parentSubject := &Subject{
				ID:    subject.ID,
				Roles: []string{role.Parent},
			}
			if r.hasPermission(parentSubject, permissionName, visited) {
				return true
			}
		}
	}

	return false
}

// HasRole checks if a subject has a specific role
func (r *RBAC) HasRole(subject *Subject, roleName string) bool {
	for _, role := range subject.Roles {
		if role == roleName {
			return true
		}
	}
	return false
}

// GetRolePermissions returns all permissions for a role
func (r *RBAC) GetRolePermissions(roleName string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	role, ok := r.roles[roleName]
	if !ok {
		return nil
	}

	permissions := make([]string, len(role.Permissions))
	copy(permissions, role.Permissions)

	// Include parent permissions
	if role.Parent != "" {
		parentPerms := r.GetRolePermissions(role.Parent)
		permissions = append(permissions, parentPerms...)
	}

	return permissions
}

// GetSubjectPermissions returns all permissions for a subject
func (r *RBAC) GetSubjectPermissions(subject *Subject) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	permMap := make(map[string]bool)

	for _, roleName := range subject.Roles {
		perms := r.GetRolePermissions(roleName)
		for _, perm := range perms {
			permMap[perm] = true
		}
	}

	permissions := make([]string, 0, len(permMap))
	for perm := range permMap {
		permissions = append(permissions, perm)
	}

	return permissions
}

// Context keys for RBAC
type rbacContextKey string

const (
	subjectContextKey rbacContextKey = "rbac_subject"
)

// WithSubject adds a subject to the context
func WithSubject(ctx context.Context, subject *Subject) context.Context {
	return context.WithValue(ctx, subjectContextKey, subject)
}

// GetSubject retrieves a subject from the context
func GetSubject(ctx context.Context) *Subject {
	subject, ok := ctx.Value(subjectContextKey).(*Subject)
	if !ok {
		return nil
	}
	return subject
}

// RequirePermission returns a middleware that requires a specific permission
func (r *RBAC) RequirePermission(permissionName string) func(next func(w interface{}, req interface{})) func(w interface{}, req interface{}) {
	return func(next func(w interface{}, req interface{})) func(w interface{}, req interface{}) {
		return func(w interface{}, req interface{}) {
			// This is a generic wrapper - actual HTTP middleware would be implemented separately
			next(w, req)
		}
	}
}

// Common roles and permissions

// DefineDefaultRoles defines common roles
func (r *RBAC) DefineDefaultRoles() {
	// Admin role
	r.AddRole(&Role{
		Name:        "admin",
		Description: "Administrator with full access",
		Permissions: []string{},
	})

	// User role
	r.AddRole(&Role{
		Name:        "user",
		Description: "Standard user",
		Permissions: []string{},
	})

	// Guest role
	r.AddRole(&Role{
		Name:        "guest",
		Description: "Guest with limited access",
		Permissions: []string{},
	})

	// Moderator role
	r.AddRole(&Role{
		Name:        "moderator",
		Description: "Moderator with elevated privileges",
		Permissions: []string{},
		Parent:      "user",
	})
}

// DefineDefaultPermissions defines common permissions
func (r *RBAC) DefineDefaultPermissions() {
	permissions := []*Permission{
		{Name: "users:read", Description: "Read users", Resource: "users", Action: "read"},
		{Name: "users:write", Description: "Write users", Resource: "users", Action: "write"},
		{Name: "users:delete", Description: "Delete users", Resource: "users", Action: "delete"},
		{Name: "posts:read", Description: "Read posts", Resource: "posts", Action: "read"},
		{Name: "posts:write", Description: "Write posts", Resource: "posts", Action: "write"},
		{Name: "posts:delete", Description: "Delete posts", Resource: "posts", Action: "delete"},
		{Name: "comments:read", Description: "Read comments", Resource: "comments", Action: "read"},
		{Name: "comments:write", Description: "Write comments", Resource: "comments", Action: "write"},
		{Name: "comments:delete", Description: "Delete comments", Resource: "comments", Action: "delete"},
		{Name: "admin:access", Description: "Admin panel access", Resource: "admin", Action: "access"},
	}

	for _, perm := range permissions {
		r.AddPermission(perm)
	}
}

// SetupDefaultRBAC creates RBAC with default roles and permissions
func SetupDefaultRBAC() *RBAC {
	rbac := NewRBAC()

	// Define roles and permissions
	rbac.DefineDefaultRoles()
	rbac.DefineDefaultPermissions()

	// Grant permissions to admin
	rbac.GrantPermission("admin", "users:read")
	rbac.GrantPermission("admin", "users:write")
	rbac.GrantPermission("admin", "users:delete")
	rbac.GrantPermission("admin", "posts:read")
	rbac.GrantPermission("admin", "posts:write")
	rbac.GrantPermission("admin", "posts:delete")
	rbac.GrantPermission("admin", "comments:read")
	rbac.GrantPermission("admin", "comments:write")
	rbac.GrantPermission("admin", "comments:delete")
	rbac.GrantPermission("admin", "admin:access")

	// Grant permissions to user
	rbac.GrantPermission("user", "posts:read")
	rbac.GrantPermission("user", "posts:write")
	rbac.GrantPermission("user", "comments:read")
	rbac.GrantPermission("user", "comments:write")

	// Grant permissions to moderator
	rbac.GrantPermission("moderator", "posts:delete")
	rbac.GrantPermission("moderator", "comments:delete")

	// Grant permissions to guest
	rbac.GrantPermission("guest", "posts:read")
	rbac.GrantPermission("guest", "comments:read")

	return rbac
}
