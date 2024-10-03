# Design Document

## How will your server use threads/goroutines? In other words, when are they created, and for what purpose?

The server is going to use goroutines to handle multiple clients subscribing at the same time. I plan to use 1 thread per client because I want to design for the scalabilty of clients. Each time a client connects to the server, the server creates a thread for the client. This thread will further handle all future communication between client and server including switching servers and streaming music.

## What data does your server need to store for each client?

For each client, the server needs to store

- Which channel is the client listening to
- Port address of the clients listening client
- Port address of the clients control client
- IP address of the client

## What data does your server need to store for each station?

The server needs to store:

- What are all the different files and songs that our server supports?
- Which clients are subscribed to each of my song channels?
- How many clients do I have; How many clients are on each channel?
- What are all the port addresses of each client(listeningPort and controlPort)

## What synchronization primitives (channels, mutexes, condition variables, etc.) will you need to have threads/goroutines communicate with each other and protect shared data?

From my understanding of channels in go, it offers a convenient way of "queueing" messages between different go routines. Therefore when clients connect and change channels, we want to send a messsage to the main server thread that is managing state through a channel. Therefore as different clients send messages to the main server thread, the channel will queue these messages and ensure that both threads are ready before making any changes. This will ensure that shared resources are not changed at the same time. The key will be to control when the server is and is not available. By controlling this, we control the flow of information through the channel.

## What happens on the server (in terms of the threads/data structures/etc you described in (1)-(4)) when a client changes stations? Thinking about this should help you answer the above questions

When a client connects, it should register itself on the state of the server. This includes a mapping from client to its port information. Once the client selects a channel, we will also update some state on the server that is tracking which song the client is listening to. This can be a client -> song mapping. We also want to update some state on the server that indicates which client is on which channel. This is a mapping from song -> [clients, clients, clients, ...]. Before we make these changes, we want to make sure that we indicate the server as unavailable while it's processing this clients channel subscription. This is to ensure that other clients aren't modifying the server's shared information at the same time. Once the state changes are finished, the server will be marked as available again, and any queued messages in the go channel will be processed similarly.
