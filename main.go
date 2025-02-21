package main

import (
	"bytes"
	"crypto/ed25519"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/lib/pq"

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

// Discord interaction types
type Interaction struct {
	Type          int             `json:"type"`
	Data          InteractionData `json:"data"`
	Token         string          `json:"token"`
	ApplicationID string          `json:"application_id"`
	GuildID       string          `json:"guild_id"`
}

type InteractionData struct {
	Name    string                  `json:"name"`
	Options []InteractionDataOption `json:"options"`
}

type InteractionDataOption struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Type  int    `json:"type"`
}

type InteractionResponse struct {
	Type int                     `json:"type"`
	Data InteractionResponseData `json:"data"`
}

type InteractionResponseData struct {
	Content string `json:"content"`
}

// Discord command registration
type Command struct {
	Name        string          `json:"name"`
	Type        int             `json:"type"`
	Description string          `json:"description"`
	Options     []CommandOption `json:"options"`
}

type CommandOption struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        int    `json:"type"`
	Required    bool   `json:"required"`
}

func registerDiscordCommands() error {
	command := Command{
		Name:        "gay",
		Type:        1,
		Description: "Changes the content of efans.gay",
		Options: []CommandOption{
			{
				Name:        "message",
				Description: "The new message to display",
				Type:        3, // STRING type
				Required:    true,
			},
		},
	}

	jsonData, err := json.Marshal(command)
	if err != nil {
		return fmt.Errorf("error marshaling command: %v", err)
	}

	url := fmt.Sprintf("https://discord.com/api/v8/applications/%s/guilds/%s/commands",
		os.Getenv("DISCORD_APPLICATION_ID"),
		os.Getenv("DISCORD_GUILD_ID"))

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Authorization", "Bot "+os.Getenv("DISCORD_BOT_TOKEN"))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, body, "", "  "); err != nil {
		log.Printf("Discord API response: status %d: %s", resp.StatusCode, string(body))
	} else {
		log.Printf("Discord API response: status %d:\n%s", resp.StatusCode, prettyJSON.String())
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("error registering command: status %d", resp.StatusCode)
	}

	return nil
}

func verifyDiscordRequest(r *http.Request) error {
	signature := r.Header.Get("X-Signature-Ed25519")
	timestamp := r.Header.Get("X-Signature-Timestamp")

	if signature == "" || timestamp == "" {
		return fmt.Errorf("missing signature headers")
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("error reading body: %v", err)
	}
	// Replace the body for later use
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	pubKeyBytes, err := hex.DecodeString(os.Getenv("DISCORD_PUBLIC_KEY"))
	if err != nil {
		return fmt.Errorf("error decoding public key: %v", err)
	}

	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("error decoding signature: %v", err)
	}

	message := []byte(timestamp + string(body))

	if !ed25519.Verify(pubKeyBytes, message, sigBytes) {
		return fmt.Errorf("invalid signature")
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
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file")
	}

	// Register Discord commands
	if err := registerDiscordCommands(); err != nil {
		log.Printf("Warning: Failed to register Discord commands: %v", err)
	}

	// Add database connection
	db, err := sql.Open("postgres", os.Getenv("POSTGRES_URL"))
	if err != nil {
		log.Fatal("Error connecting to database:", err)
	}
	defer db.Close()

	// Test the connection
	if err := db.Ping(); err != nil {
		log.Fatal("Error pinging database:", err)
	}

	// Initialize MOTD store with persisted or default message
	motdStore := &MOTDStore{
		message:     "does citadel usually make money off these things?",
		lastUpdated: time.Now().Unix(),
	}

	// Try to load saved MOTD
	if data, err := os.ReadFile("data/efans.txt"); err == nil {
		motdStore.Set(string(data))
	}

	// Create file server handler for static files
	fs := http.FileServer(http.Dir("public"))

	// Update Discord webhook handler to save to database
	http.HandleFunc("/discord-webhook", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Read the body once for verification and storage
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("Error reading body: %v", err)
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		// Store raw JSON for later use
		rawBody := json.RawMessage(body)
		r.Body = io.NopCloser(bytes.NewBuffer(body)) // Replace the body for later use
		// Verify the request is from Discord
		if err := verifyDiscordRequest(r); err != nil {
			log.Printf("Discord verification failed: %v", err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var interaction Interaction
		if err := json.NewDecoder(bytes.NewBuffer(body)).Decode(&interaction); err != nil {
			log.Printf("Error decoding interaction: %v", err)
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// Verify guild ID
		if interaction.GuildID != os.Getenv("DISCORD_GUILD_ID") {
			log.Printf("Unauthorized guild ID: %s", interaction.GuildID)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Log the interaction for debugging
		log.Printf("Received interaction type: %d", interaction.Type)

		// Handle PING interaction type (type 1)
		if interaction.Type == 1 {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(InteractionResponse{
				Type: 1, // PONG
			})
			return
		}

		// Handle the set command
		if interaction.Type == 2 && interaction.Data.Name == "gay" && len(interaction.Data.Options) > 0 {
			newMessage := interaction.Data.Options[0].Value

			if err := motdStore.Set(newMessage); err != nil {
				log.Printf("Error saving MOTD: %v", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			// Save to database
			if err := saveRequestToDB(db, newMessage, rawBody); err != nil {
				log.Printf("Error saving to database: %v", err)
				// Continue processing even if database save fails
			}

			// Respond to Discord
			response := InteractionResponse{
				Type: 4, // CHANNEL_MESSAGE_WITH_SOURCE
				Data: InteractionResponseData{
					Content: fmt.Sprintf("Updated https://efans.gay message to: %s", newMessage),
				},
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}

		http.Error(w, "Unknown command", http.StatusBadRequest)
	})

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
