package main

import (
	"net/http"
	"regexp"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

type SignupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	OrgName  string `json:"orgName"`
	OrgSlug  string `json:"orgSlug"`
}

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func HandleSignup(e *core.RequestEvent) error {
	var data SignupRequest

	if err := e.BindBody(&data); err != nil {
		return apis.NewBadRequestError("Invalid request data", err)
	}

	if data.Email == "" || data.Password == "" || data.OrgName == "" || data.OrgSlug == "" {
		return apis.NewBadRequestError("All fields are required", nil)
	}

	if len(data.Password) < 8 {
		return apis.NewBadRequestError("Password must be at least 8 characters", nil)
	}

	if !slugPattern.MatchString(data.OrgSlug) || len(data.OrgSlug) < 3 || len(data.OrgSlug) > 15 {
		return apis.NewBadRequestError(
			"Slug must be 3-15 characters, lowercase letters, numbers, and hyphens only", nil)
	}

	// Check slug uniqueness
	existing, _ := e.App.FindFirstRecordByData("orgs", "slug", data.OrgSlug)
	if existing != nil {
		return apis.NewBadRequestError("An organization with this slug already exists", nil)
	}

	return e.App.RunInTransaction(func(txApp core.App) error {
		// Create user
		usersCollection, err := txApp.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		user := core.NewRecord(usersCollection)
		user.Set("email", data.Email)
		user.Set("name", data.Email[:indexOf(data.Email, '@')])
		user.Set("emailVisibility", true)
		user.Set("verified", true)
		user.SetPassword(data.Password)

		if err := txApp.Save(user); err != nil {
			return apis.NewBadRequestError("Failed to create account", err)
		}

		// Create organization
		orgsCollection, err := txApp.FindCollectionByNameOrId("orgs")
		if err != nil {
			return err
		}

		org := core.NewRecord(orgsCollection)
		org.Set("name", data.OrgName)
		org.Set("slug", data.OrgSlug)

		if err := txApp.Save(org); err != nil {
			return apis.NewBadRequestError("Failed to create organization", err)
		}

		// Link user to org as admin
		userOrgCollection, err := txApp.FindCollectionByNameOrId("user_org")
		if err != nil {
			return err
		}

		userOrg := core.NewRecord(userOrgCollection)
		userOrg.Set("user", user.Id)
		userOrg.Set("org", org.Id)
		userOrg.Set("role", "admin")

		if err := txApp.Save(userOrg); err != nil {
			return apis.NewBadRequestError("Failed to link user to organization", err)
		}

		return e.JSON(http.StatusOK, map[string]any{
			"userId":  user.Id,
			"orgSlug": data.OrgSlug,
		})
	})
}

func indexOf(s string, c byte) int {
	for i := range s {
		if s[i] == c {
			return i
		}
	}
	return len(s)
}
