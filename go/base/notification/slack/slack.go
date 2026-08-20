// Package slack provides a thin wrapper around the slack-go client for sending
// notifications to Slack channels.
package slack

import (
	"github.com/slack-go/slack"
	"go.uber.org/config"
	"go.uber.org/fx"
)

const configKey = "slack"

// Config holds Slack client configuration loaded from YAML.
type Config struct {
	Token string `yaml:"token"`
}

// Client wraps the slack-go API client.
type Client struct {
	api *slack.Client
}

// Module provides FX dependency injection for the Slack client.
var Module = fx.Options(
	fx.Provide(newConfig),
	fx.Provide(NewClient),
)

func newConfig(provider config.Provider) (Config, error) {
	conf := Config{}
	err := provider.Get(configKey).Populate(&conf)
	return conf, err
}

// NewClient creates a new Slack client from the given config.
// Returns nil without error if token is not configured — Slack notifications will be skipped.
func NewClient(conf Config) (*Client, error) {
	if conf.Token == "" {
		return nil, nil
	}
	return &Client{api: slack.New(conf.Token)}, nil
}

// PostMessage sends a plain-text message to the given Slack channel or user ID.
func (c *Client) PostMessage(channel, text string) error {
	_, _, err := c.api.PostMessage(channel, slack.MsgOptionText(text, false))
	return err
}

// PostBlocks sends a Block Kit message to the given Slack channel or user ID.
func (c *Client) PostBlocks(channel string, blocks ...slack.Block) error {
	_, _, err := c.api.PostMessage(channel, slack.MsgOptionBlocks(blocks...))
	return err
}

// GetUserIDByEmail looks up a Slack user ID by email address.
func (c *Client) GetUserIDByEmail(email string) (string, error) {
	user, err := c.api.GetUserByEmail(email)
	if err != nil {
		return "", err
	}
	return user.ID, nil
}