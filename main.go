package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/go-redis/redis"
)

var (
	Token     = os.Getenv("DISCORD_BOT_TOKEN")
	GuildID   = os.Getenv("DISCORD_GUILD_ID")
	RedisAddr = os.Getenv("REDIS_ADDR")
)

var client *redis.Client

type Affiliate struct {
	InviterID string
	Invitees  map[string]bool
}

func main() {
	client = redis.NewClient(&redis.Options{
		Addr: RedisAddr,
		// Password: "",
		DB: 0,
	})

	_, err := client.Ping().Result()
	if err != nil {
		fmt.Println("Error connecting to Redis: ", err)
		return
	}

	dg, err := discordgo.New("Bot " + Token)
	if err != nil {
		fmt.Println("Error creating Discord session: ", err)
		return
	}

	dg.AddHandler(messageCreate)
	dg.AddHandler(memberJoin)
	dg.AddHandler(memberLeave)

	err = dg.Open()
	if err != nil {
		fmt.Println("Error opening Discord session: ", err)
		return
	}

	fmt.Println("Bot is now running. Press CTRL+C to exit.")
	<-make(chan struct{})
	return
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	if strings.HasPrefix(m.Content, "/createlink") {
		createAffiliateLink(s, m)
	}
}

func createAffiliateLink(s *discordgo.Session, m *discordgo.MessageCreate) {
	i, err := s.ChannelInviteCreate(m.ChannelID, discordgo.Invite{
		MaxAge:    0,
		MaxUses:   0,
		Temporary: false,
		Unique:    true,
	})
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "Error creating invite link.")
		return
	}

	err = client.HSet(i.Code, "inviterID", m.Author.ID).Err()
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "Error storing invite data.")
		return
	}
	err = client.HSet(i.Code, "uses", 0).Err()
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "Error storing invite data.")
		return
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Invite link: https://discord.gg/%s", i.Code))
}

func memberJoin(s *discordgo.Session, m *discordgo.GuildMemberAdd) {
	invites, err := s.GuildInvites(GuildID)
	if err != nil {
		fmt.Println("Error retrieving guild invites: ", err)
		return
	}

	for _, invite := range invites {
		inviterID, err := client.HGet(invite.Code, "inviterID").Result()
		if err != nil {
			fmt.Println("Error retrieving invite data: ", err)
			continue
		}

		uses, _ := client.HGet(invite.Code, "uses").Int()
		if invite.Uses > uses {
			client.HSet(invite.Code, "uses", invite.Uses)
			client.SAdd("affiliate:"+inviterID+":invitees", m.User.ID)
			client.Set("user:"+m.User.ID+":inviter", inviterID, 0)
			// add the users discord name to the inviter's list of invitees
			break
		}
	}
}

func memberLeave(s *discordgo.Session, m *discordgo.GuildMemberRemove) {
	inviterID, err := client.Get("user:" + m.User.ID + ":inviter").Result()
	if err != nil {
		fmt.Println("Error retrieving inviter data: ", err)
		return
	}

	client.SRem("affiliate:"+inviterID+":invitees", m.User.ID)
}
