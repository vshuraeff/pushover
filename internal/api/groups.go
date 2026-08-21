package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"unicode/utf8"
)

// group represents a Pushover group.
type Group struct {
	Key  string `json:"group"`
	Name string `json:"name"`
}

// group member represents a user assigned to a Pushover group.
type GroupMember struct {
	User     string `json:"user"`
	Device   string `json:"device"`
	Memo     string `json:"memo"`
	Disabled bool   `json:"disabled"`
}

// group detail contains a group's name and members.
type GroupDetail struct {
	Name  string        `json:"name"`
	Users []GroupMember `json:"users"`
}

// create group creates a group and returns its key.
func (c *Client) CreateGroup(ctx context.Context, appToken, name string) (string, error) {
	values := url.Values{"token": {appToken}, "name": {name}}
	response, err := c.PostForm(ctx, "/1/groups.json", values)
	if err != nil {
		return "", err
	}

	var result struct {
		Group string `json:"group"`
	}
	if err := json.Unmarshal(response.Body, &result); err != nil {
		return "", fmt.Errorf("decode create group response: %w", err)
	}
	return result.Group, nil
}

// list groups returns all groups available to an application.
func (c *Client) ListGroups(ctx context.Context, appToken string) ([]Group, error) {
	response, err := c.Get(ctx, "/1/groups.json", url.Values{"token": {appToken}})
	if err != nil {
		return nil, err
	}

	var result struct {
		Groups []Group `json:"groups"`
	}
	if err := json.Unmarshal(response.Body, &result); err != nil {
		return nil, fmt.Errorf("decode list groups response: %w", err)
	}
	return result.Groups, nil
}

// get group returns the members and name for a group.
func (c *Client) GetGroup(ctx context.Context, appToken, groupKey string) (GroupDetail, error) {
	response, err := c.Get(ctx, "/1/groups/"+groupKey+".json", url.Values{"token": {appToken}})
	if err != nil {
		return GroupDetail{}, err
	}

	var result GroupDetail
	if err := json.Unmarshal(response.Body, &result); err != nil {
		return GroupDetail{}, fmt.Errorf("decode group response: %w", err)
	}
	return result, nil
}

// add group user adds a user to a group.
func (c *Client) AddGroupUser(ctx context.Context, appToken, groupKey, user, device, memo string) error {
	if utf8.RuneCountInString(memo) > 200 {
		return fmt.Errorf("group member memo exceeds 200 runes")
	}

	values := groupUserValues(appToken, user, device)
	if memo != "" {
		values.Set("memo", memo)
	}
	return c.postGroup(ctx, "/1/groups/"+groupKey+"/add_user.json", values)
}

// remove group user removes a user's group membership.
func (c *Client) RemoveGroupUser(ctx context.Context, appToken, groupKey, user, device string) error {
	return c.postGroup(ctx, "/1/groups/"+groupKey+"/remove_user.json", groupUserValues(appToken, user, device))
}

// disable group user disables a user's group membership.
func (c *Client) DisableGroupUser(ctx context.Context, appToken, groupKey, user, device string) error {
	return c.postGroup(ctx, "/1/groups/"+groupKey+"/disable_user.json", groupUserValues(appToken, user, device))
}

// enable group user enables a user's group membership.
func (c *Client) EnableGroupUser(ctx context.Context, appToken, groupKey, user, device string) error {
	return c.postGroup(ctx, "/1/groups/"+groupKey+"/enable_user.json", groupUserValues(appToken, user, device))
}

// rename group changes a group's name.
func (c *Client) RenameGroup(ctx context.Context, appToken, groupKey, name string) error {
	return c.postGroup(ctx, "/1/groups/"+groupKey+"/rename.json", url.Values{"token": {appToken}, "name": {name}})
}

func (c *Client) postGroup(ctx context.Context, path string, values url.Values) error {
	_, err := c.PostForm(ctx, path, values)
	return err
}

func groupUserValues(appToken, user, device string) url.Values {
	values := url.Values{"token": {appToken}, "user": {user}}
	if device != "" {
		values.Set("device", device)
	}
	return values
}
