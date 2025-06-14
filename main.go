package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	irc "github.com/thoj/go-ircevent"
)

type Config struct {
	Server   string `json:"server"`
	Nick     string `json:"nick"`
	Channel  string `json:"channel"`
	NickServ struct {
		Password string `json:"password,omitempty"`
	} `json:"nickserv,omitempty"`
	AuthorizedNicks map[string]bool `json:"authorized_nicks,omitempty"`
}

type Showtime struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	DateTime  time.Time `json:"datetime"`
	CreatedBy string    `json:"created_by"`
}

type CinemaBot struct {
	conn      *irc.Connection
	config    Config
	showtimes map[string]Showtime
}

func NewCinemaBot(configFile string) (*CinemaBot, error) {
	bot := &CinemaBot{
		showtimes: make(map[string]Showtime),
	}

	// Load config
	if err := bot.loadConfig(configFile); err != nil {
		return nil, fmt.Errorf("failed to load config: %v", err)
	}

	// Setup IRC connection
	bot.conn = irc.IRC(bot.config.Nick, bot.config.Nick)
	bot.conn.VerboseCallbackHandler = true
	bot.conn.Debug = false

	// Add event handlers
	bot.setupHandlers()

	return bot, nil
}

func (bot *CinemaBot) loadConfig(configFile string) error {
	if configFile == "" {
		// Default config if no file specified
		bot.config = Config{
			Server:  "irc.snoonet.org:6667",
			Nick:    "sdcinemabot",
			Channel: "#stopdrinkingcinema",
		}
		return nil
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &bot.config)
}

func (bot *CinemaBot) setupHandlers() {
	bot.conn.AddCallback("001", func(e *irc.Event) {
		// If NickServ password is configured, identify
		if bot.config.NickServ.Password != "" {
			bot.conn.Privmsg("NickServ", fmt.Sprintf("IDENTIFY %s", bot.config.NickServ.Password))
			time.Sleep(2 * time.Second) // Wait for identification
		}

		// Join channel
		bot.conn.Join(bot.config.Channel)
		log.Printf("Joined %s", bot.config.Channel)
	})

	bot.conn.AddCallback("PRIVMSG", func(e *irc.Event) {
		message := e.Message()
		nick := e.Nick
		host := e.Host
		//log.Printf("Message from %s!%s: %s", nick, host, message)

		// Only respond to messages in our channel
		if e.Arguments[0] != bot.config.Channel {
			return
		}

		// Handle ;showtime command
		if strings.HasPrefix(message, ";showtime") {
			if bot.authorizedShowtimeCommand(nick, host) {
				bot.handleShowtimeCommand(message, nick)
			} else {
				bot.conn.Privmsg(bot.config.Channel, fmt.Sprintf("%s: You are not authorized to use this command.", nick))
				log.Printf("Unauthorized showtime command attempt by %s!%s", nick, host)
			}
		}

		// Handle ;nextmovie command (available to everyone)
		if strings.HasPrefix(message, ";nextmovie") {
			bot.handleNextMovieCommand()
		}
	})
}

func (bot *CinemaBot) authorizedShowtimeCommand(nick, host string) bool {
	if bot.config.AuthorizedNicks[nick] && host == "user/"+nick {
		return true
	}

	return false
}

func (bot *CinemaBot) handleNextMovieCommand() {
	now := time.Now()
	var nextShowtime *Showtime
	var shortestDuration time.Duration

	// Find the next upcoming showtime
	for _, showtime := range bot.showtimes {
		if showtime.DateTime.After(now) {
			duration := showtime.DateTime.Sub(now)
			if nextShowtime == nil || duration < shortestDuration {
				nextShowtime = &showtime
				shortestDuration = duration
			}
		}
	}

	if nextShowtime == nil {
		bot.conn.Privmsg(bot.config.Channel, "No upcoming movies scheduled!")
		return
	}

	// Format the time until the movie in a human-friendly way
	timeMessage := bot.formatTimeUntil(shortestDuration)

	message := fmt.Sprintf("%s, %s is playing!", timeMessage, nextShowtime.Title)
	bot.conn.Privmsg(bot.config.Channel, message)
	log.Printf("Next movie response sent: %s", message)
}

// Also fix the createShowtime function to ensure consistent timezone handling
func (bot *CinemaBot) createShowtime(args []string, nick string) {
	var id, title, date string
	var hours, minutes, seconds, month, day, year int
	var err error

	// Parse arguments (same as before)
	for _, part := range args[2:] { // Skip ";showtime" and "-create"
		if strings.HasPrefix(part, "-id=") {
			id = strings.Trim(strings.TrimPrefix(part, "-id="), "\"")
		} else if strings.HasPrefix(part, "-title=") {
			title = strings.Trim(strings.TrimPrefix(part, "-title="), "\"")
		} else if strings.HasPrefix(part, "-hour=") {
			hours, err = strconv.Atoi(strings.Trim(strings.TrimPrefix(part, "-hour="), "\""))
			if err != nil {
				bot.conn.Privmsg(bot.config.Channel, "Invalid hour value.")
				return
			}
		} else if strings.HasPrefix(part, "-hours=") {
			hours, err = strconv.Atoi(strings.Trim(strings.TrimPrefix(part, "-hours="), "\""))
			if err != nil {
				bot.conn.Privmsg(bot.config.Channel, "Invalid hours value.")
				return
			}
		} else if strings.HasPrefix(part, "-minutes=") {
			minutes, err = strconv.Atoi(strings.Trim(strings.TrimPrefix(part, "-minutes="), "\""))
			if err != nil {
				bot.conn.Privmsg(bot.config.Channel, "Invalid minutes value.")
				return
			}
		} else if strings.HasPrefix(part, "-minute=") {
			minutes, err = strconv.Atoi(strings.Trim(strings.TrimPrefix(part, "-minute="), "\""))
			if err != nil {
				bot.conn.Privmsg(bot.config.Channel, "Invalid minute value.")
				return
			}
		} else if strings.HasPrefix(part, "-second=") {
			seconds, err = strconv.Atoi(strings.Trim(strings.TrimPrefix(part, "-second="), "\""))
			if err != nil {
				bot.conn.Privmsg(bot.config.Channel, "Invalid second value.")
				return
			}
		} else if strings.HasPrefix(part, "-seconds=") || strings.HasPrefix(part, "-sec=") {
			var secStr string
			if strings.HasPrefix(part, "-seconds=") {
				secStr = strings.Trim(strings.TrimPrefix(part, "-seconds="), "\"")
			} else {
				secStr = strings.Trim(strings.TrimPrefix(part, "-sec="), "\"")
			}
			seconds, err = strconv.Atoi(secStr)
			if err != nil {
				bot.conn.Privmsg(bot.config.Channel, "Invalid seconds value.")
				return
			}
		} else if strings.HasPrefix(part, "-month=") {
			month, err = strconv.Atoi(strings.Trim(strings.TrimPrefix(part, "-month="), "\""))
			if err != nil {
				bot.conn.Privmsg(bot.config.Channel, "Invalid month value.")
				return
			}
		} else if strings.HasPrefix(part, "-day=") {
			day, err = strconv.Atoi(strings.Trim(strings.TrimPrefix(part, "-day="), "\""))
			if err != nil {
				bot.conn.Privmsg(bot.config.Channel, "Invalid day value.")
				return
			}
		} else if strings.HasPrefix(part, "-year=") {
			year, err = strconv.Atoi(strings.Trim(strings.TrimPrefix(part, "-year="), "\""))
			if err != nil {
				bot.conn.Privmsg(bot.config.Channel, "Invalid year value.")
				return
			}
		} else if strings.HasPrefix(part, "-date=") {
			date = strings.Trim(strings.TrimPrefix(part, "-date="), "\"")
		}
	}

	// Validate required fields
	if id == "" || title == "" {
		bot.conn.Privmsg(bot.config.Channel, "Required: -id=\"id\" -title=\"title\"")
		return
	}

	// Check if ID already exists
	if _, exists := bot.showtimes[id]; exists {
		bot.conn.Privmsg(bot.config.Channel, fmt.Sprintf("Showtime with ID '%s' already exists.", id))
		return
	}

	// Create datetime - use current time as base if not all fields specified
	now := time.Now()
	if year == 0 {
		year = now.Year()
	}
	if month == 0 {
		month = int(now.Month())
	}
	if day == 0 {
		day = now.Day()
	}

	var datetime time.Time
	if date != "" {
		const layout = "2006-01-02 15:04:05"
		datetime, err = time.Parse(layout, date)
		if err != nil {
			bot.conn.Privmsg(bot.config.Channel, "Invalid date format. Example: 2025-07-02 15:04:05")
			return
		}
	} else {
		datetime = time.Date(year, time.Month(month), day, hours, minutes, seconds, 0, now.Location())
	}

	// Ensure datetime is in the future
	if datetime.Before(now) {
		bot.conn.Privmsg(bot.config.Channel, "Showtime must be in the future.")
		return
	}

	// Create and store the showtime
	showtime := Showtime{
		ID:        id,
		Title:     title,
		DateTime:  datetime,
		CreatedBy: nick,
	}

	bot.showtimes[id] = showtime

	timeStr := datetime.Format("2006-01-02 15:04:05")
	bot.conn.Privmsg(bot.config.Channel,
		fmt.Sprintf("Created showtime: [%s] %s - %s", id, title, timeStr))

	// Debug logging
	log.Printf("Created showtime [%s]: %s at %s (created by %s)", id, title, timeStr, nick)
}

func (bot *CinemaBot) formatTimeUntil(duration time.Duration) string {
	totalSeconds := int(duration.Seconds())

	if totalSeconds < 60 {
		if totalSeconds == 1 {
			return "In 1 second"
		}
		return fmt.Sprintf("In %d seconds", totalSeconds)
	}

	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60

	var parts []string

	if hours > 0 {
		if hours == 1 {
			parts = append(parts, "1 hour")
		} else {
			parts = append(parts, fmt.Sprintf("%d hours", hours))
		}
	}

	if minutes > 0 {
		if minutes == 1 {
			parts = append(parts, "1 minute")
		} else {
			parts = append(parts, fmt.Sprintf("%d minutes", minutes))
		}
	}

	if seconds > 0 {
		if seconds == 1 {
			parts = append(parts, "1 second")
		} else {
			parts = append(parts, fmt.Sprintf("%d seconds", seconds))
		}
	}

	return "In " + strings.Join(parts, ", ")
}

func (bot *CinemaBot) handleShowtimeCommand(message, nick string) {
	// Parse the command more carefully to handle quoted arguments
	args := bot.parseArgs(message)
	if len(args) < 2 {
		bot.conn.Privmsg(bot.config.Channel, "Usage: ;showtime -list | -create [options] | -delete=\"id\"")
		return
	}

	// Check if any argument starts with -delete
	var hasDelete bool
	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "-delete") {
			hasDelete = true
			break
		}
	}

	switch {
	case args[1] == "-list":
		bot.listShowtimes()
	case hasDelete:
		bot.deleteShowtime(args, nick)
	case args[1] == "-create":
		bot.createShowtime(args, nick)
	default:
		bot.conn.Privmsg(bot.config.Channel, "Usage: ;showtime -list | -create [options] | -delete=\"id\"")
	}
}

func (bot *CinemaBot) listShowtimes() {
	if len(bot.showtimes) == 0 {
		bot.conn.Privmsg(bot.config.Channel, "No showtimes scheduled.")
		return
	}

	// Sort showtimes by datetime for better display
	var sortedShowtimes []Showtime
	for _, showtime := range bot.showtimes {
		sortedShowtimes = append(sortedShowtimes, showtime)
	}

	sort.Slice(sortedShowtimes, func(i, j int) bool {
		return sortedShowtimes[i].DateTime.Before(sortedShowtimes[j].DateTime)
	})

	bot.conn.Privmsg(bot.config.Channel, "Scheduled showtimes:")
	for _, showtime := range sortedShowtimes {
		timeStr := showtime.DateTime.Format("2006-01-02 15:04:05")
		msg := fmt.Sprintf("[%s] %s - %s (by %s)",
			showtime.ID, showtime.Title, timeStr, showtime.CreatedBy)
		bot.conn.Privmsg(bot.config.Channel, msg)
	}
}

// parseArgs parses command arguments, handling quoted strings properly
func (bot *CinemaBot) parseArgs(message string) []string {
	var args []string
	var current strings.Builder
	inQuotes := false
	escaped := false

	for _, char := range message {
		switch char {
		case '\\':
			if escaped {
				current.WriteRune(char)
				escaped = false
			} else {
				escaped = true
			}
		case '"':
			if escaped {
				current.WriteRune(char)
				escaped = false
			} else {
				if inQuotes {
					// End of quoted section - always add the current token, even if empty
					args = append(args, current.String())
					current.Reset()
				}
				inQuotes = !inQuotes
			}
		case ' ', '\t':
			if escaped {
				// Handle escaped spaces and tabs
				current.WriteRune(char)
				escaped = false
			} else if inQuotes {
				current.WriteRune(char)
			} else {
				// Outside quotes - this is a separator
				if current.Len() > 0 {
					args = append(args, current.String())
					current.Reset()
				}
			}
		case 't':
			if escaped {
				// Convert \t to actual tab character
				current.WriteRune('\t')
				escaped = false
			} else {
				current.WriteRune(char)
			}
		case 'n':
			if escaped {
				// Convert \n to actual newline character
				current.WriteRune('\n')
				escaped = false
			} else {
				current.WriteRune(char)
			}
		case 'r':
			if escaped {
				// Convert \r to actual carriage return character
				current.WriteRune('\r')
				escaped = false
			} else {
				current.WriteRune(char)
			}
		default:
			if escaped {
				// If we have an escape followed by something we don't handle specially,
				// keep the backslash
				current.WriteRune('\\')
				escaped = false
			}
			current.WriteRune(char)
		}
	}

	// Handle any remaining content
	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}

func (bot *CinemaBot) deleteShowtime(args []string, nick string) {
	var id string

	// Parse -delete="id" format
	for _, part := range args {
		if strings.HasPrefix(part, "-delete=") {
			id = strings.Trim(strings.TrimPrefix(part, "-delete="), "\"")
			break
		}
	}

	if id == "" {
		bot.conn.Privmsg(bot.config.Channel, "Usage: ;showtime -delete=\"id\"")
		return
	}

	showtime, exists := bot.showtimes[id]
	if !exists {
		bot.conn.Privmsg(bot.config.Channel, fmt.Sprintf("Showtime with ID '%s' not found.", id))
		return
	}

	// Only allow deletion by creator or admin check could be added here
	if showtime.CreatedBy != nick {
		bot.conn.Privmsg(bot.config.Channel, "You can only delete showtimes you created.")
		return
	}

	delete(bot.showtimes, id)
	bot.conn.Privmsg(bot.config.Channel, fmt.Sprintf("Deleted showtime: %s", id))
}

func (bot *CinemaBot) Connect() error {
	err := bot.conn.Connect(bot.config.Server)
	if err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}

	bot.conn.Loop()
	return nil
}

// startHealthCheckServer starts a simple HTTP server for health checks
func startHealthCheckServer() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	log.Printf("Starting health check server on :8000")
	go func() {
		if err := http.ListenAndServe(":8000", nil); err != nil {
			log.Printf("Health check server error: %v", err)
		}
	}()
}

func main() {
	configFile := flag.String("config", "bot_config.json", "Path to config file (optional)")
	flag.Parse()

	bot, err := NewCinemaBot(*configFile)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	// Start health check server
	startHealthCheckServer()

	log.Printf("Starting CinemaBot...")
	log.Printf("Server: %s", bot.config.Server)
	log.Printf("Nick: %s", bot.config.Nick)
	log.Printf("Channel: %s", bot.config.Channel)

	if err := bot.Connect(); err != nil {
		log.Fatalf("Connection failed: %v", err)
	}
}
