/* Automatically generated by utils/generate-command-help.rb, do not edit. */

#ifndef __REDIS_HELP_H
#define __REDIS_HELP_H

static char *commandGroups[] = {
    "generic",
    "pubsub",
    "connection",
    "server"
};

struct commandHelp {
  char *name;
  char *params;
  char *summary;
  int group;
  char *since;
} commandHelp[] = {
    { "AUTH",
    "password",
    "Authenticate to the server",
    2,
    "1.0.0" },
    { "CLIENT KILL",
    "ip:port",
    "Kill the connection of a client",
    3,
    "1.0.0" },
    { "CLIENT LIST",
    "-",
    "Get the list of client connections",
    3,
    "1.0.0" },
    { "EXEC",
    "-",
    "Execute all commands",
    0,
    "1.0.0" },
    { "INFO",
    "-",
    "Get information and statistics about the server",
    3,
    "1.0.0" },
    { "MONITOR",
    "-",
    "Listen for all requests received by the server in real time",
    3,
    "1.0.0" },
    { "PING",
    "-",
    "Ping the server",
    3,
    "1.0.0" },
    { "PSUBSCRIBE",
    "pattern [pattern ...]",
    "Listen for messages published to channels matching the given patterns",
    1,
    "1.0.0" },
    { "PUBLISH",
    "channel message",
    "Post a message to a channel",
    1,
    "1.0.0" },
    { "PUNSUBSCRIBE",
    "[pattern [pattern ...]]",
    "Stop listening for messages posted to channels matching the given patterns",
    1,
    "1.0.0" },
    { "QUIT",
    "-",
    "Close the connection",
    2,
    "1.0.0" },
    { "SHUTDOWN",
    "[NOSAVE] [SAVE]",
    "Synchronously save the dataset to disk and then shut down the server",
    3,
    "1.0.0" },
    { "SUBSCRIBE",
    "channel [channel ...]",
    "Listen for messages published to the given channels",
    1,
    "1.0.0" },
    { "TIME",
    "-",
    "Return the current server time",
    3,
    "1.0.0" },
    { "UNSUBSCRIBE",
    "[channel [channel ...]]",
    "Stop listening for messages posted to the given channels",
    1,
    "1.0.0"}
};

#endif
