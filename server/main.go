package main

import (
	"log"
	"os"
	"server/kimba"
	"server/session"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

func connectDiscord(token string) *discordgo.Session {
	if token == "" {
		log.Println("No DISCORD_TOKEN set, Discord bridge disabled")
		return nil
	}
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalf("Discord session error: %v", err)
	}
	dg.Identify.Intents = discordgo.IntentsGuildMessages
	if err := dg.Open(); err != nil {
		log.Fatalf("Discord open error: %v", err)
	}
	log.Println("Discord bot connected")
	return dg
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	godotenv.Load()

	token := os.Getenv("DISCORD_TOKEN")
	guildID := os.Getenv("DISCORD_GUILD_ID")

	dg := connectDiscord(token)
	if dg != nil {
		defer dg.Close()
	}

	go func() {
		if err := kimba.ListenAndServe("0.0.0.0:25026"); err != nil {
			log.Fatalf("Kimba proxy error: %v", err)
		}
	}()

	if err := session.ListenAndServe("0.0.0.0:25025", dg, guildID); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
