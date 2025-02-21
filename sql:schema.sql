CREATE TABLE "posts" (
    "id" integer GENERATED ALWAYS AS IDENTITY,
    "body" text NOT NULL,
    "created_at" timestamptz NOT NULL DEFAULT now(),
    "discord_message_id" text UNIQUE,
    "discord_channel_id" text NOT NULL,
    "user_id" text NOT NULL,
    "username" text NOT NULL,
    "x_post_id" text,
    PRIMARY KEY ("id")
); 