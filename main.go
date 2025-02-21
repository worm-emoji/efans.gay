package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/lib/pq"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

type PageData struct {
	MOTD        string
	LastUpdated int64
}

// MOTDStore handles thread-safe access to the MOTD
type MOTDStore struct {
	sync.RWMutex
	message     string
	lastUpdated int64
}

func (s *MOTDStore) Get() string {
	s.RLock()
	defer s.RUnlock()
	return s.message
}

func (s *MOTDStore) GetLastUpdated() int64 {
	s.RLock()
	defer s.RUnlock()
	return s.lastUpdated
}

func (s *MOTDStore) Set(message string) error {
	s.Lock()
	defer s.Unlock()
	s.message = message
	s.lastUpdated = time.Now().Unix()

	// Ensure data directory exists
	if err := os.MkdirAll("data", 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %v", err)
	}

	// Save to file
	if err := os.WriteFile("data/efans.txt", []byte(message), 0644); err != nil {
		return fmt.Errorf("failed to write MOTD to file: %v", err)
	}
	return nil
}

func saveRequestToDB(db *sql.DB, body string, request json.RawMessage) error {
	_, err := db.Exec(`
		INSERT INTO posts (body, request, created_at)
		VALUES ($1, $2, NOW())
	`, body, []byte(request))

	if err != nil {
		return fmt.Errorf("error inserting into database: %v", err)
	}

	return nil
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("Error loading .env file: %v", err)
	}

	log.Printf("Initializing Discord bot with application ID: %s", os.Getenv("DISCORD_APPLICATION_ID"))
	
	// Initialize Discord bot
	discord, err := discordgo.New("Bot " + os.Getenv("DISCORD_BOT_TOKEN"))
	if err != nil {
		log.Fatalf("Error creating Discord session: %v", err)
	}

	// Add connection state change logging
	discord.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Discord bot is ready! Logged in as: %s#%s", s.State.User.Username, s.State.User.Discriminator)
		log.Printf("Bot is present in %d guilds", len(s.State.Guilds))
		for _, guild := range s.State.Guilds {
			log.Printf("Connected to guild: %s (ID: %s)", guild.Name, guild.ID)
		}
	})

	discord.AddHandler(func(s *discordgo.Session, c *discordgo.Connect) {
		log.Printf("Discord connection established to gateway")
	})

	discord.AddHandler(func(s *discordgo.Session, d *discordgo.Disconnect) {
		log.Printf("Discord connection lost, attempting to reconnect...")
	})

	discord.AddHandler(func(s *discordgo.Session, r *discordgo.Resumed) {
		log.Printf("Discord connection resumed")
	})

	// Initialize database connection
	log.Printf("Connecting to database...")
	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}
	defer db.Close()
	log.Printf("Database connection established")

	// Initialize MOTD store
	motdStore := &MOTDStore{}
	
	// Load initial MOTD from file
	if data, err := os.ReadFile("data/efans.txt"); err == nil {
		motdStore.Set(string(data))
	}

	// Create static file server
	fs := http.FileServer(http.Dir("public"))

	// Set up Discord bot handlers
	discord.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		// Check if interaction is from the allowed guild
		allowedGuildID := os.Getenv("DISCORD_GUILD_ID")
		if i.GuildID != allowedGuildID {
			log.Printf("Ignoring interaction from unauthorized guild: %s", i.GuildID)
			return
		}

		switch i.Type {
		case discordgo.InteractionPing:
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponsePong,
			})

		case discordgo.InteractionApplicationCommand:
			if i.ApplicationCommandData().Name == "gay" {
				options := i.ApplicationCommandData().Options
				if len(options) > 0 {
					newMessage := options[0].StringValue()

					if err := motdStore.Set(newMessage); err != nil {
						log.Printf("Error saving MOTD: %v", err)
						return
					}

					// Save to database
					if err := saveRequestToDB(db, newMessage, []byte(i.Token)); err != nil {
						log.Printf("Error saving to database: %v", err)
					}

					s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
						Type: discordgo.InteractionResponseChannelMessageWithSource,
						Data: &discordgo.InteractionResponseData{
							Content: fmt.Sprintf("Updated https://efans.gay message to: %s", newMessage),
						},
					})
				}
			}
		}
	})

	// Register the slash command
	command := &discordgo.ApplicationCommand{
		Name:        "gay",
		Description: "Update the message on efans.gay",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "message",
				Description: "The new message to display",
				Required:    true,
			},
		},
	}

	// Open Discord connection
	log.Printf("Opening Discord connection...")
	if err := discord.Open(); err != nil {
		log.Fatalf("Error opening Discord connection: %v", err)
	}
	defer discord.Close()

	// Register the command with Discord
	log.Printf("Registering slash command 'gay' for guild ID: %s", os.Getenv("DISCORD_GUILD_ID"))
	_, err = discord.ApplicationCommandCreate(
		discord.State.User.ID,
		os.Getenv("DISCORD_GUILD_ID"),
		command,
	)
	if err != nil {
		log.Printf("Error creating slash command: %v", err)
	} else {
		log.Printf("Slash command 'gay' registered successfully")
	}

	// Regular web handlers
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			tmpl, err := template.ParseFiles(filepath.Join("public", "index.html"))
			if err != nil {
				log.Printf("Error parsing template: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			data := PageData{
				MOTD:        motdStore.Get(),
				LastUpdated: motdStore.GetLastUpdated(),
			}

			err = tmpl.Execute(w, data)
			if err != nil {
				log.Printf("Error executing template: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			return
		}

		fs.ServeHTTP(w, r)
	})

	http.HandleFunc("/last-updated", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "%d", motdStore.GetLastUpdated())
	})

	log.Println("Server starting on http://localhost:4331")
	if err := http.ListenAndServe("127.0.0.1:4331", nil); err != nil {
		log.Fatal(err)
	}
}
