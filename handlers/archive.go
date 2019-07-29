package handlers

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/bwmarrin/discordgo"

	. "github.com/unswpcsoc/PCSocBot/commands"
)

const (
	historyLim  = 2000
	archiveChan = "462063414408249376" // TODO: change
	scrollEmoji = string(0x1f4dc)
)

var (
	history = []*qelem{}
)

type qelem struct {
	cID string
	mID string
}

func enqueue(cid, mid string) {
	history = append(history, &qelem{cid, mid})
	if len(history) > historyLim {
		history = history[1:len(history)]
	}
}

type Archive struct {
	NilCommand
	Index int `arg:"index"`
}

func NewArchive() *Archive { return &Archive{} }

func (a *Archive) Aliases() []string { return []string{"archive"} }

func (a *Archive) Desc() string { return "Generates an embed for archiving a message" }

func (a *Archive) Roles() []string { return []string{"mod"} }

func (a *Archive) MsgHandle(ses *discordgo.Session, msg *discordgo.Message) (*CommandSend, error) {
	var err error

	if len(history) == 0 {
		return nil, errors.New("no logged messages have been reacted with " + scrollEmoji)
	}

	// check index
	if a.Index >= len(history) || a.Index < 0 {
		return nil, errors.New("index not in range")
	}

	cid := history[len(history)-a.Index].cID
	mid := history[len(history)-a.Index].mID

	// get archive target
	var arc *discordgo.Message
	arc, err = ses.State.Message(cid, mid)
	if err != nil {
		arc, err = ses.ChannelMessage(cid, mid)
		if err != nil {
			return nil, err
		}
	}

	var cha *discordgo.Channel
	cha, err = ses.State.Channel(arc.ChannelID)
	if err != nil {
		ses.Channel(arc.ChannelID)
		if err != nil {
			return nil, err
		}
	}

	// craft message
	out := &discordgo.MessageSend{
		Content: "",
		Tts:     false,
	}

	// add images
	if len(arc.Attachments) > 0 {
		// get response, assuming only 1 attachment
		url := arc.Attachments[0].URL
		splits := strings.Split(url, ".")
		format := splits[len(splits)-1]

		resp, err := http.Get(url)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		// read into buffer
		buf, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		// attach to our message
		out.File = &discordgo.File{
			Name:        "archive." + format,
			ContentType: "image/" + format,
			Reader:      bytes.NewReader(buf),
		}
	}

	// generate archive embed
	out.Embed = &discordgo.MessageEmbed{
		Author: &discordgo.MessageEmbedAuthor{
			// hotlink
			URL: "https://discordapp.com/channels/" + arc.GuildID +
				"/" + cid + "/" + mid,
			IconURL: arc.Author.AvatarURL(""),
			Name:    arc.Author.String(),
		},

		Description: arc.Content,

		Footer: &discordgo.MessageEmbedFooter{
			Text: "Archived message from " + cha.Name + " | " + string(arc.Timestamp),
		},

		Color: ses.State.UserColor(arc.Author.ID, cid),
	}

	// send to archive channel
	ses.ChannelMessageSendComplex(archiveChan, out)

	return NewSimpleSend(msg.ChannelID, "Archived message!"), nil
}

func initArchive(ses *discordgo.Session) {
	ses.AddHandler(func(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
		react := r.MessageReaction
		if react.Emoji.Name == scrollEmoji {
			enqueue(react.ChannelID, react.MessageID)
		}
	})
}
