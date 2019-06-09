// Packange commands implements a command interface for PCSocBot
// with helper structs and funcs for sending discordgo messages,
// and high-level abstractions of buntdb

package commands

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/tidwall/buntdb"

	"github.com/unswpcsoc/PCSocBot/utils"
)

var (
	/* package global db */
	db *buntdb.DB

	/* send errors */
	ErrSendLimit = errors.New("message exceeds send limit of 2000 characters")

	/* db errors */
	ErrDBClosed   = errors.New("db not open, use DBOpen()")
	ErrDBOpen     = errors.New("db already open")
	ErrDBValueNil = errors.New("cannot set nil value")
	ErrDBKeyEmpty = errors.New("cannot set value with empty key")

	/* storer errors */
	ErrStorerNil = errors.New("storer method received nil")
)

const (
	MESSAGE_LIMIT = 2000
	SEND_LIMIT    = 10
)

// Command The command interface that all commands implement
type Command interface {
	Names() []string // Names of commands (used in routing)
	Desc() string    // Description of command
	Roles() []string // Roles required (should all be lowercased)
	Chans() []string // Channels required (should all be lowercased)

	MsgHandle(*discordgo.Session, *discordgo.Message, []string) (*CommandSend, error) // Handler for MessageCreate event
}

// Send is a helper struct that buffers things commands need to send
type CommandSend struct {
	data      []*discordgo.MessageSend `json:"messages"`
	channelid string                   `json:"channelid"`
}

// NewSend Returns a send struct.
func NewSend(cid string) *CommandSend {
	return &CommandSend{
		make([]*discordgo.MessageSend, SEND_LIMIT),
		cid,
	}
}

// NewSimpleSend Returns a send struct with the message content filled in.
func NewSimpleSend(cid string, msg string) *CommandSend {
	send := &discordgo.MessageSend{
		Content: msg,
		Embed:   nil,
		Tts:     false,
		Files:   nil,
		File:    nil,
	}
	return &CommandSend{
		data:      []*discordgo.MessageSend{send},
		channelid: cid,
	}
}

// AddSimpleMessage Adds another simple message to be sent.
func (c *CommandSend) AddSimpleMessage(msg string) {
	send := &discordgo.MessageSend{
		Content: msg,
		Embed:   nil,
		Tts:     false,
		Files:   nil,
		File:    nil,
	}
	c.data = append(c.data, send)
}

// AddEmbedMessage Adds an embed message to be sent.
func (c *CommandSend) AddEmbedMessage(emb *discordgo.MessageEmbed) {
	send := &discordgo.MessageSend{
		Content: "",
		Embed:   emb,
		Tts:     false,
		Files:   nil,
		File:    nil,
	}
	c.data = append(c.data, send)
}

// AddMessageSend Adds a discordgo MessageSend
func (c *CommandSend) AddMessageSend(send *discordgo.MessageSend) {
	c.data = append(c.data, send)
}

// Send Sends the messages a command returns while also checking message length
func (c *CommandSend) Send(s *discordgo.Session) error {
	// Get the stuff out of BeegYoshi and send it into the server
	for _, data := range c.data {
		if utils.Strlen(data) > MESSAGE_LIMIT {
			return fmt.Errorf("Send: following message exceeds limit\n%#v", data)
		}
		s.ChannelMessageSendComplex(c.channelid, data)
	}
	return nil
}

/* db stuff */

// Storer is the interface for structs that will be stored into the db
// NB: You MUST export all fields in a Storer, otherwise the JSON Marshaller will freak out
// there are workarounds, but they require more effort than we need.
// Read https://stackoverflow.com/a/49372417 if you're interested
type Storer interface {
	Index() string                    // Determines db index
	Unmarshal(string) (Storer, error) // Unmarshals a string json encoding into the storer's type
}

// Open opens the db at the given path
func DBOpen(path string) error {
	if db != nil {
		return ErrDBClosed
	}
	var err error
	db, err = buntdb.Open(path)
	return err
}

// Close closes the db
func DBClose() error {
	if db == nil {
		return ErrDBOpen
	}
	err := db.Close()
	if err != nil {
		return err
	}
	db = nil
	return nil
}

// DBSet is a command method that sets an arbitrary key/value pair into the database under the current command's index.
// note: values cannot be nil
func DBSet(s Storer, key string) (previous string, replaced bool, err error) {
	// Assert db open so we can rollback transactions on later errors
	if db == nil {
		return "", false, ErrDBClosed
	}
	if s == nil {
		return "", false, ErrStorerNil
	}
	if len(key) == 0 {
		return "", false, ErrDBKeyEmpty
	}

	// Begin RW transaction
	tx, err := db.Begin(true)
	if err != nil {
		tx.Rollback()
		return "", false, err
	}

	// Marshal storer
	mar, err := json.Marshal(s)
	if err != nil {
		tx.Rollback()
		return "", false, err
	}

	// Set marshalled key/value pair
	pre, rep, err := tx.Set(s.Index()+":"+key, string(mar), nil)
	if err != nil {
		tx.Rollback()
		return "", false, err
	}

	// Commit changes
	err = tx.Commit()
	if err != nil {
		tx.Rollback()
		return "", false, err
	}

	return pre, rep, nil
}

// DBGet gets the Storer at the given key. Ignores expiry.
func DBGet(s Storer, key string) (Storer, error) {
	if db == nil {
		return nil, ErrDBClosed
	}
	if s == nil {
		return nil, ErrStorerNil
	}

	// Open RO Transaction, defer rollback
	tx, err := db.Begin(false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Get Storer
	got, err := tx.Get(s.Index()+":"+key, true)
	if err != nil {
		return nil, err
	}

	// Unmarshal Storer and return it
	return s.Unmarshal(got)
}