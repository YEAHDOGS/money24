# AI apps

Money 24 asks the AI apps you already pay for, through their own command-line tools, signed in with
your subscriptions or their free tiers. Money 24 pays for no AI and holds no AI keys. The apps run on
the machine that runs the desk, as the Windows user who owns the `Money24` logon task, because that is
where they are signed in.

| AI | Tool | Pays | One-time step on the desk machine |
|---|---|---|---|
| Claude | Claude Code | your Claude plan | install Claude Code, run `claude` once and sign in |
| ChatGPT | Codex | your ChatGPT plan | install Codex, run `codex login`, choose Sign in with ChatGPT |
| Gemini | Antigravity (`agy`) | your Google account | install Antigravity, open it once and sign in with Google |
| OpenCode | OpenCode | its free models | `npm install -g opencode-ai`; no account needed |

Google no longer lets individuals use Gemini CLI; Antigravity is how Gemini answers here. Grok has no
free route, so it is not used.

## Check the desk machine

After signing in, from the repo on the desk machine:

```
cd engine
go run ./cmd/m24d --ai-check
```

It asks every app a tiny question at once, prints who answered and how fast, then puts the README's FAA
story to them and prints what two of them agreed on. At least two must answer. The pocket's **AI** tab
shows the same, refreshed every half hour, with the one step that fixes an app that stops answering.

## How a question is asked

- **No tools, no web, no saved session, an empty folder.** Each ask runs in its own empty folder under
  the data folder (`ai-work/`), so no project instructions are read and no files can be touched.
- **A fixed answer shape.** Every app must answer one JSON object matching a JSON Schema: Claude Code,
  Codex and Antigravity enforce it; OpenCode gets the schema in its prompt.
- **News is quoted as untrusted data.** The prompt tells each app to ignore instructions inside the
  story, and the story can't close its own quote.
- **Answers never trade.** They are untrusted input. A strategy reads them, picks contracts from the real
  option chain with local code, and passes the play through the same risk limits as every other trade.

## Speed

News decays in seconds, so every app runs on a fast model at low effort, all at once, and the desk
decides as soon as **two agree**: the same direction and at least one ticker in common. The slower apps
keep answering for their record but are never waited for. Their consensus is the direction they share
and only the tickers every one of them named.

Measured on 2026-11-22, one FAA question to each app, cold start included:

| AI | Answer |
|---|---|
| Claude (Haiku, low effort) | about 2 to 3 s |
| ChatGPT (Codex, low effort) | about 3.5 to 5 s |
| OpenCode (free model) | about 3 to 7 s |
| Gemini (Flash Low) | about 4 to 6 s |
| **Two agreeing** | **about 3.5 to 5 s** |

Most of each answer is the tool starting up. Keeping each tool running between questions (Claude Code
and Antigravity can take question after question on stdin; OpenCode can attach to a running server) is
the next step to bring this toward the model's own time, about 1.5 to 3 s.

## Routes

| Route | Who | What |
|---|---|---|
| `GET /api/ai` | anyone the runner lets read | each app's state, last answer time, and the last 20 stories asked |
| `POST /api/ai/test` | the desk on this machine | asks every app the tiny question now |
| `POST /api/ai/impact` `{"news": "..."}` | the desk on this machine | puts one story to every app and returns when two agree |

Asks spend your plans, so only the desk on the machine itself may make them.
