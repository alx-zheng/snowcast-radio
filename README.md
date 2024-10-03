# Snowcast

## Summary

Snowcast is a network radio station that supports multiple client-to-server connections, allowing users to tune into various music stations. Each station broadcasts a unique song, and clients can subscribe to their preferred station, freely switching between channels. Upon subscribing, clients start receiving the live audio stream from the station.

### Server Design

The server is built to support an unlimited number of clients and performs several tasks upon startup:

Continuous Broadcasting: Each music station broadcasts continuously, regardless of whether any clients are tuned in. The server constantly monitors client subscriptions and ensures that each client receives the stream from their selected station.

TCP Connection Handling: The server opens a TCP port and listens for incoming client connections. When a new client connects, the server spawns a dedicated thread to handle all interactions with that client, such as receiving messages and updating the server’s state.

REPL Interface: After initializing the broadcasting and client connection systems, the server provides a REPL (Read-Eval-Print Loop) interface, accepting the commands p, p [file], and q. The p command outputs a snapshot of the server's current state, including active stations, songs, and connected clients.

### Client Design

When a client connects to the server, it sets up a TCP socket and spawns a thread dedicated to receiving messages from the server. The client initiates a handshake by sending a Hello message, and upon receiving a Welcome response, the connection is established. The client then offers a REPL interface, allowing the user to switch between music stations by entering station numbers, which triggers the client to send a Set Station message to the server.

### Listener Design

The listener component establishes a UDP port that continuously outputs incoming data from the subscribed station's stream, ensuring smooth and uninterrupted playback for the user.

### Design Decisions

#### Server side

Server-Side
Thread-per-Client Model: Each client is handled by its own thread, allowing the server to efficiently manage large numbers of clients without bottlenecks. For example, if a single thread were responsible for handling all clients on a station, it could become overloaded with too many users.

Server State Management: The server maintains a serverInfo object that maps client addresses to their respective station subscriptions and client information. It also tracks which stations are playing which songs and which clients are subscribed to each station. To ensure thread safety while multiple clients read from or update this shared state, a mutex is used.

Efficient Mutex Handling: The mutex locks the serverInfo object in two scenarios:

- When a client sends a Set Station message, the server locks the state, updates the necessary mappings, and then releases the mutex once the update is complete.
- Before each broadcast round, the server temporarily locks serverInfo to take a snapshot of the current state. The mutex is released immediately after copying, allowing the server to continue broadcasting without locking the state for an extended period. This approach prevents delays in accepting new clients or updating their state during long broadcast operations.

Station Threads: Each music station is handled by a dedicated thread, which periodically reads 1024 bytes from its corresponding MP3 file. These bytes are sent to all clients subscribed to the station at intervals of 62.5 milliseconds, maintaining the desired data rate. This approach ensures the song continues to progress even when no clients are tuned in, and it guarantees that all listeners receive the same portion of the stream.

Announce Messages:

- An Announce message is sent immediately when the server receives a Set Station message from a client.
- For periodic announcements, such as when a song repeats, the station thread handles this. When the thread detects the end of a song, it resets the song to the beginning and sends an Announce message to all clients on that station.

Asynchronous State Updates: The server uses a goroutine to update the serverInfo object when a new client joins. This non-blocking approach ensures the server can send an Announce message promptly, while still updating the server’s state in the background. This reduces the delay between a client sending a Set Station message and receiving confirmation.

#### Client side

On startup, the client sets up a TCP connection and starts a thread to listen for messages such as Welcome and Announce from the server. Once initialized, the client provides a REPL interface that allows users to select stations by number, sending the appropriate Set Station messages to the server.