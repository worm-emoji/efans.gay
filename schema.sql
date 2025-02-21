CREATE TABLE "posts" (
    "id" integer GENERATED ALWAYS AS IDENTITY,
    "body" text NOT NULL,
    "created_at" timestamptz NOT NULL DEFAULT now(),
    "request" jsonb,
    PRIMARY KEY ("id")
);