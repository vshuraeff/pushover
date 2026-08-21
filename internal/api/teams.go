package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// team member is a user returned by the Pushover teams endpoint.
type TeamMember struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Email         string            `json:"email"`
	Administrator bool              `json:"administrator"`
	Devices       []json.RawMessage `json:"devices"`
}

// team user options configures an invitation sent by AddTeamUser.
type TeamUserOptions struct {
	Name          string
	Password      string
	Instant       bool
	Administrator bool
	Group         string
}

// list team returns the team name and its members.
func (c *Client) ListTeam(ctx context.Context, teamToken string) (string, []TeamMember, error) {
	response, err := c.Get(ctx, "/1/teams.json", url.Values{"token": {teamToken}})
	if err != nil {
		return "", nil, fmt.Errorf("list team: %w", err)
	}

	var result teamResponse
	if err := json.Unmarshal(response.Body, &result); err != nil {
		return "", nil, fmt.Errorf("decode team response: %w", err)
	}
	return result.Name, result.Users, nil
}

// add team user invites an email address to a team.
func (c *Client) AddTeamUser(ctx context.Context, teamToken, email string, options TeamUserOptions) error {
	if email == "" {
		return fmt.Errorf("team user email is required")
	}

	values := url.Values{"token": {teamToken}, "email": {email}}
	if options.Name != "" {
		values.Set("name", options.Name)
	}
	if options.Password != "" {
		values.Set("password", options.Password)
	}
	if options.Instant {
		values.Set("instant", strconv.FormatBool(options.Instant))
	}
	if options.Administrator {
		values.Set("admin", strconv.FormatBool(options.Administrator))
	}
	if options.Group != "" {
		values.Set("group", options.Group)
	}

	if _, err := c.PostForm(ctx, "/1/teams/add_user.json", values); err != nil {
		return fmt.Errorf("add team user: %w", err)
	}
	return nil
}

// revoke team invitation revokes an outstanding invitation by email address.
func (c *Client) RevokeTeamInvitation(ctx context.Context, teamToken, email string) error {
	return c.postTeamEmail(ctx, "/1/teams/revoke_invitation.json", teamToken, email, "revoke team invitation")
}

// remove team user removes a team member by email address.
func (c *Client) RemoveTeamUser(ctx context.Context, teamToken, email string) error {
	return c.postTeamEmail(ctx, "/1/teams/remove_user.json", teamToken, email, "remove team user")
}

type teamResponse struct {
	Name  string       `json:"name"`
	Users []TeamMember `json:"users"`
}

func (c *Client) postTeamEmail(ctx context.Context, path, teamToken, email, operation string) error {
	if email == "" {
		return fmt.Errorf("team user email is required")
	}
	if _, err := c.PostForm(ctx, path, url.Values{"token": {teamToken}, "email": {email}}); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}
