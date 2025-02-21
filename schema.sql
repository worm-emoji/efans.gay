CREATE TABLE "posts" (
    "id" integer GENERATED ALWAYS AS IDENTITY,
    "body" text NOT NULL,
    "created_at" timestamptz NOT NULL DEFAULT now(),
    "discord_message_id" text UNIQUE,
    "user_id" text NOT NULL,
    "username" text NOT NULL,
    PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS reactions (
    post_id INTEGER REFERENCES posts(id),
    user_id TEXT NOT NULL,
    emoji TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (post_id, user_id, emoji)
);

CREATE INDEX idx_discord_message_id ON posts(discord_message_id);