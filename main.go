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
	"sync"
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
	bot.conn.VerboseCallbackHandler = false
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

	var mu sync.Mutex

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
		//log.Printf("Message from %s!%s: %s", nick, host, message) // Removed message logging

		// Only respond to messages in our channel
		if e.Arguments[0] != bot.config.Channel {
			return
		}

		mu.Lock()
		defer mu.Unlock()

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

		if strings.HasPrefix(message, ";date") {
			bot.handleDateCommand()
		}
	})
}

func (bot *CinemaBot) handleDateCommand() {
	// Write the current date in UTC
	now := time.Now().UTC()
	bot.conn.Privmsg(bot.config.Channel, fmt.Sprintf("Current time (UTC): %s", now.Format("2006-01-02 15:04:05 MST")))
}

func (bot *CinemaBot) authorizedShowtimeCommand(nick, host string) bool {
	if bot.config.AuthorizedNicks[nick] && host == "user/"+nick {
		return true
	}

	return false
}

func (bot *CinemaBot) handleNextMovieCommand() {
	now := time.Now().UTC() // Always use UTC
	var nextShowtime *Showtime
	var currentShowtime *Showtime
	var shortestDuration time.Duration
	var shortestCurrentDuration time.Duration

	// Find the next upcoming showtime and the most recent past showtime
	for _, showtime := range bot.showtimes {
		// All times are stored in UTC
		if showtime.DateTime.After(now) {
			// Future showtime
			duration := showtime.DateTime.Sub(now)
			if nextShowtime == nil || duration < shortestDuration {
				nextShowtime = &showtime
				shortestDuration = duration
			}
		} else {
			// Past showtime - find the most recent one
			duration := now.Sub(showtime.DateTime)
			if currentShowtime == nil || duration < shortestCurrentDuration {
				currentShowtime = &showtime
				shortestCurrentDuration = duration
			}
		}
	}

	// Prioritize current/recently started movie over future ones
	if currentShowtime != nil {
		// Show how long the movie has been playing
		timeMessage := bot.formatTimeSince(shortestCurrentDuration)
		message := fmt.Sprintf("%s into %s", timeMessage, currentShowtime.Title)
		bot.conn.Privmsg(bot.config.Channel, message)
		log.Printf("Current movie response sent: %s", message)
		return
	}

	// If no current movie, show next upcoming one
	if nextShowtime != nil {
		timeMessage := bot.formatTimeUntil(shortestDuration)
		message := fmt.Sprintf("%s, %s is playing!", timeMessage, nextShowtime.Title)
		bot.conn.Privmsg(bot.config.Channel, message)
		log.Printf("Next movie response sent: %s", message)
		return
	}

	// No movies at all
	bot.conn.Privmsg(bot.config.Channel, "No movies scheduled!")
}

func (bot *CinemaBot) createShowtime(args []string, nick string) {
	var id, title, date string
	var hours, minutes, seconds, month, day, year int
	var err error

	// Parse arguments
	for _, part := range args[2:] { // Skip ";showtime" and "-create"
		if strings.HasPrefix(part, "-id=") {
			id = strings.Trim(strings.TrimPrefix(part, "-id="), "\"")
		} else if strings.HasPrefix(part, "-title=") {
			title = strings.Trim(strings.TrimPrefix(part, "-title="), "\"")
		} else if strings.HasPrefix(part, "-hour=") || strings.HasPrefix(part, "-hours=") {
			var hourStr string
			if strings.HasPrefix(part, "-hour=") {
				hourStr = strings.Trim(strings.TrimPrefix(part, "-hour="), "\"")
			} else {
				hourStr = strings.Trim(strings.TrimPrefix(part, "-hours="), "\"")
			}
			hours, err = strconv.Atoi(hourStr)
			if err != nil || hours < 0 || hours > 23 {
				bot.conn.Privmsg(bot.config.Channel, "Invalid hour value (must be 0-23).")
				return
			}
		} else if strings.HasPrefix(part, "-minute=") || strings.HasPrefix(part, "-minutes=") {
			var minStr string
			if strings.HasPrefix(part, "-minute=") {
				minStr = strings.Trim(strings.TrimPrefix(part, "-minute="), "\"")
			} else {
				minStr = strings.Trim(strings.TrimPrefix(part, "-minutes="), "\"")
			}
			minutes, err = strconv.Atoi(minStr)
			if err != nil || minutes < 0 || minutes > 59 {
				bot.conn.Privmsg(bot.config.Channel, "Invalid minute value (must be 0-59).")
				return
			}
		} else if strings.HasPrefix(part, "-second=") || strings.HasPrefix(part, "-seconds=") || strings.HasPrefix(part, "-sec=") {
			var secStr string
			if strings.HasPrefix(part, "-second=") {
				secStr = strings.Trim(strings.TrimPrefix(part, "-second="), "\"")
			} else if strings.HasPrefix(part, "-seconds=") {
				secStr = strings.Trim(strings.TrimPrefix(part, "-seconds="), "\"")
			} else {
				secStr = strings.Trim(strings.TrimPrefix(part, "-sec="), "\"")
			}
			seconds, err = strconv.Atoi(secStr)
			if err != nil || seconds < 0 || seconds > 59 {
				bot.conn.Privmsg(bot.config.Channel, "Invalid second value (must be 0-59).")
				return
			}
		} else if strings.HasPrefix(part, "-month=") {
			month, err = strconv.Atoi(strings.Trim(strings.TrimPrefix(part, "-month="), "\""))
			if err != nil || month < 1 || month > 12 {
				bot.conn.Privmsg(bot.config.Channel, "Invalid month value (must be 1-12).")
				return
			}
		} else if strings.HasPrefix(part, "-day=") {
			day, err = strconv.Atoi(strings.Trim(strings.TrimPrefix(part, "-day="), "\""))
			if err != nil || day < 1 || day > 31 {
				bot.conn.Privmsg(bot.config.Channel, "Invalid day value (must be 1-31).")
				return
			}
		} else if strings.HasPrefix(part, "-year=") {
			year, err = strconv.Atoi(strings.Trim(strings.TrimPrefix(part, "-year="), "\""))
			if err != nil || year < 1900 || year > 2100 {
				bot.conn.Privmsg(bot.config.Channel, "Invalid year value (must be 1900-2100).")
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

	// Create datetime
	now := time.Now().UTC()
	var datetime time.Time

	if date != "" {
		// Parse full date string - support multiple formats, always in UTC
		formats := []string{
			"2006-01-02 15:04:05",
			"2006-01-02 15:04",
			"01-02-2006 15:04:05",
			"01-02-2006 15:04",
			"2006/01/02 15:04:05",
			"2006/01/02 15:04",
		}

		var parseErr error
		for _, format := range formats {
			datetime, parseErr = time.Parse(format, date)
			if parseErr == nil {
				// Convert to UTC if not already
				datetime = datetime.UTC()
				break
			}
		}

		if parseErr != nil {
			bot.conn.Privmsg(bot.config.Channel, "Invalid date format. Supported formats: 2006-01-02 15:04:05, 01-02-2006 15:04:05, 2006/01/02 15:04:05")
			return
		}
	} else {
		// Use current time as base if not all fields specified
		if year == 0 {
			year = now.Year()
		}
		if month == 0 {
			month = int(now.Month())
		}
		if day == 0 {
			day = now.Day()
		}

		// Create date in UTC and validate it's valid (handles leap years, month boundaries, etc.)
		datetime = time.Date(year, time.Month(month), day, hours, minutes, seconds, 0, time.UTC)

		// Check if the date is valid by comparing with what we intended
		if datetime.Year() != year || int(datetime.Month()) != month || datetime.Day() != day {
			bot.conn.Privmsg(bot.config.Channel, "Invalid date (check month/day combination and leap year).")
			return
		}
	}

	// Create and store the showtime
	showtime := Showtime{
		ID:        id,
		Title:     title,
		DateTime:  datetime, // Store in UTC
		CreatedBy: nick,
	}

	bot.showtimes[id] = showtime

	timeStr := datetime.Format("2006-01-02 15:04:05 MST")
	bot.conn.Privmsg(bot.config.Channel,
		fmt.Sprintf("Created showtime: [%s] %s - %s", id, title, timeStr))

	// Debug logging
	log.Printf("Created showtime [%s]: %s at %s (created by %s)", id, title, timeStr, nick)
}

func (bot *CinemaBot) formatTimeUntil(duration time.Duration) string {
	// Round to nearest second to avoid showing negative durations due to microsecond differences
	totalSeconds := int(duration.Round(time.Second).Seconds())

	if totalSeconds <= 0 {
		return "Now"
	}

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

	if seconds > 0 && hours == 0 { // Only show seconds if less than an hour
		if seconds == 1 {
			parts = append(parts, "1 second")
		} else {
			parts = append(parts, fmt.Sprintf("%d seconds", seconds))
		}
	}

	return "In " + strings.Join(parts, ", ")
}

func (bot *CinemaBot) formatTimeSince(duration time.Duration) string {
	// Round to nearest second
	totalSeconds := int(duration.Round(time.Second).Seconds())

	if totalSeconds <= 0 {
		return "Just started"
	}

	if totalSeconds < 60 {
		if totalSeconds == 1 {
			return "1 second"
		}
		return fmt.Sprintf("%d seconds", totalSeconds)
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

	if seconds > 0 && hours == 0 { // Only show seconds if less than an hour
		if seconds == 1 {
			parts = append(parts, "1 second")
		} else {
			parts = append(parts, fmt.Sprintf("%d seconds", seconds))
		}
	}

	return strings.Join(parts, ", ")
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
		// Display time in UTC
		timeStr := showtime.DateTime.Format("2006-01-02 15:04:05 MST")
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
	log.Printf("Timezone: UTC")

	if err := bot.Connect(); err != nil {
		log.Fatalf("Connection failed: %v", err)
	}
}
