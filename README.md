# CinemaBot2

CinemaBot2 is an IRC bot for managing and announcing movie showtimes in a channel. It supports scheduling, listing, and deleting showtimes, as well as announcing the next or currently playing movie.

## Features

- Add, list, and delete movie showtimes (with authorization)
- Announce the next upcoming or currently playing movie
- Health check HTTP endpoint for monitoring
- In-memory storage (no persistent database)

## Deployment

### Docker

1. **Build the Docker image:**
   ```sh
   docker build -t sdcinemabot .
   ```

2. **Run the container:**
   ```sh
   docker run -p 8000:8000 sdcinemabot
   ```

   The bot will connect to the IRC server and join the configured channel. The health check server will be available at `http://localhost:8000/health`.

### Manual (Go)

1. **Install dependencies:**
   ```sh
   go mod download
   ```

2. **Build and run:**
   ```sh
   go build -o cinemabot2 .
   ./cinemabot2
   ```

   By default, the bot will use `bot_config.json` in the current directory.

## Configuration

Create a `bot_config.json` file in the working directory. Example:

```json
{
  "server": "irc.snoonet.org:6667",
  "channel": "#stopdrinkingcinema",
  "nick": "sdcinemabot",
  "nickserv": {
    "password": ""
  },
  "authorized_nicks": {
    "infinitehazlep": true,
    "chickenhips": true,
    "Eriks": true,
    "jade36": true
  }
}
```

- `server`: IRC server address.
- `channel`: Channel to join.
- `nick`: Bot nickname.
- `nickserv.password`: (optional) NickServ password for authentication.
- `authorized_nicks`: Map of nicks allowed to use showtime management commands.

You can specify a different config file with:
```sh
./cinemabot2 -config=custom_config.json
```

## Usage

### IRC Commands

- **List showtimes** (anyone):
  ```
  ;showtime -list
  ```

- **Create a showtime** (authorized users only):
  ```
  ;showtime -create -id="movie1" -title="A Movie" -hours="19" -minutes="0" -seconds="0" -month="6" -day="13" -year="2025"
  ```
  Or using a date string:
  ```
  ;showtime -create -id="movie2" -title="Another Movie" -date="2025-07-02 15:04:05"
  ```

- **Delete a showtime** (only creator can delete):
  ```
  ;showtime -delete="movie1"
  ```

- **Announce next/current movie** (anyone):
  ```
  ;nextmovie
  ```

- **Show current date (UTC)**:
  ```
  ;date
  ```

## Health Check

A simple HTTP health check server runs on port 8000:
- `GET /` or `GET /health` returns `OK`.

## Notes

- All showtimes are stored in memory and will be lost if the bot restarts.
- Only users listed in `authorized_nicks` can create or delete showtimes.
- The bot must be able to connect to the specified IRC server and channel.

---

**License:** MIT (see [LICENSE](LICENSE) if