package main

import (
	"database/sql"
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

func savePostToDB(db *sql.DB, body string, userID string, username string) (int64, error) {
	var id int64
	err := db.QueryRow(`
		INSERT INTO posts (body, user_id, username, created_at)
		VALUES ($1, $2, $3, NOW())
		RETURNING id
	`, body, userID, username).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf("error inserting into database: %v", err)
	}

	return id, nil
}

func updateMessageID(db *sql.DB, postID int64, messageID string) error {
	_, err := db.Exec(`
		UPDATE posts 
		SET discord_message_id = $1
		WHERE id = $2
	`, messageID, postID)
	return err
}

func getPostIDFromMessageID(db *sql.DB, messageID string) (int64, error) {
	var postID int64
	err := db.QueryRow(`
		SELECT id FROM posts 
		WHERE discord_message_id = $1
	`, messageID).Scan(&postID)
	return postID, err
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
	db, err := sql.Open("postgres", os.Getenv("POSTGRES_URL"))
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

					// Save to database with user info
					postID, err := savePostToDB(
						db,
						newMessage,
						i.Member.User.ID,
						i.Member.User.Username,
					)
					if err != nil {
						log.Printf("Error saving to database: %v", err)
					}

					err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
						Type: discordgo.InteractionResponseChannelMessageWithSource,
						Data: &discordgo.InteractionResponseData{
							Content: fmt.Sprintf("Updated https://efans.gay message to: %s", newMessage),
						},
					})
					if err != nil {
						log.Printf("Error responding to interaction: %v", err)
						return
					}

					// Get the message ID and update the database
					msg, err := s.InteractionResponse(i.Interaction)
					if err == nil {
						if err := updateMessageID(db, postID, msg.ID); err != nil {
							log.Printf("Error updating message ID: %v", err)
						}
					}
				}
			}
		}
	})

	// Update the reaction handlers to use the database lookup
	discord.AddHandler(func(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
		postID, err := getPostIDFromMessageID(db, r.MessageID)
		if err != nil {
			return // Message not found or error
		}

		// Store the reaction in the database
		_, err = db.Exec(`
			INSERT INTO reactions (post_id, user_id, emoji, created_at)
			VALUES ($1, $2, $3, NOW())
			ON CONFLICT (post_id, user_id, emoji) DO NOTHING
		`, postID, r.UserID, r.Emoji.MessageFormat())

		if err != nil {
			log.Printf("Error storing reaction: %v", err)
		}
	})

	discord.AddHandler(func(s *discordgo.Session, r *discordgo.MessageReactionRemove) {
		postID, err := getPostIDFromMessageID(db, r.MessageID)
		if err != nil {
			return // Message not found or error
		}

		// Remove the reaction from the database
		_, err = db.Exec(`
			DELETE FROM reactions 
			WHERE post_id = $1 
			AND user_id = $2 
			AND emoji = $3
		`, postID, r.UserID, r.Emoji.MessageFormat())

		if err != nil {
			log.Printf("Error removing reaction: %v", err)
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
