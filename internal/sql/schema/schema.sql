CREATE TABLE teams
(
    id         UUID PRIMARY KEY      DEFAULT gen_random_uuid(),
    name       VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE users
(
    id         UUID PRIMARY KEY      DEFAULT gen_random_uuid(),
    email      VARCHAR(255) NOT NULL UNIQUE,
    first_name VARCHAR(255) NOT NULL,
    last_name  VARCHAR(255) NOT NULL,
    password   VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE team_users
(
    team_id    UUID NOT NULL REFERENCES teams (id),
    user_id    UUID NOT NULL REFERENCES users (id),
    created_at TIMESTAMPTZ DEFAULT now(),

    PRIMARY KEY (team_id, user_id)
);

CREATE TABLE services
(
    id         UUID PRIMARY KEY      DEFAULT gen_random_uuid(),
    name       VARCHAR(255) NOT NULL,
    endpoint   VARCHAR(255) NOT NULL,
    type       VARCHAR(20)  NOT NULL,
    interval   INTEGER      NOT NULL,
    enabled    BOOLEAN      NOT NULL DEFAULT true,
    team_id    UUID         NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE checks
(
    id          UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    service_id  UUID        NOT NULL REFERENCES services (id) ON DELETE CASCADE,
    success     BOOLEAN     NOT NULL,
    status_code INTEGER,
    latency     INTEGER     NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TYPE notification_channel_type AS ENUM ('email', 'discord');
CREATE TYPE notification_status AS ENUM ('pending', 'sent', 'failed');

CREATE TABLE notification_channels
(
    id         UUID PRIMARY KEY                   DEFAULT gen_random_uuid(),
    team_id    UUID                      NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    type       notification_channel_type NOT NULL,
    config     JSONB                     NOT NULL,
    created_at TIMESTAMPTZ               NOT NULL DEFAULT now()
);

CREATE TABLE notification_rules
(
    id         UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    service_id UUID        NOT NULL REFERENCES services (id) ON DELETE CASCADE,
    channel_id UUID        NOT NULL REFERENCES notification_channels (id) ON DELETE CASCADE,
    threshold  INTEGER     NOT NULL DEFAULT 1,
    enabled    BOOLEAN     NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE notification_history
(
    id         UUID PRIMARY KEY             DEFAULT gen_random_uuid(),
    rule_id    UUID                NOT NULL REFERENCES notification_rules (id) ON DELETE CASCADE,
    channel_id UUID                NOT NULL REFERENCES notification_channels (id) ON DELETE CASCADE,
    check_id   UUID                NOT NULL REFERENCES checks (id) ON DELETE CASCADE,
    status     notification_status NOT NULL DEFAULT 'pending',
    sent_at    TIMESTAMPTZ         NOT NULL DEFAULT now()
);
