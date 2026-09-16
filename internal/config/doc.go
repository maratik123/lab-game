// Package config loads and validates lab-game's runtime configuration from
// three disjoint sources, each the exclusive owner of one domain — no key
// may be set through a second source, and there is no override chain
// between them:
//
//   - the process environment supplies secrets (the bot token, the
//     database DSN), runtime settings (the Bot API base URL, the allowed
//     chat ids, and the Telegram client's optional-with-default retry and
//     rate-limit tuning), and the balance-file and world-set file-system
//     paths;
//   - the balance YAML file (LAB_GAME_BALANCE_PATH) supplies every game
//     constant — stamina, combat dice, door and monster-budget curves,
//     shop rates — with no compiled-in fallback for any of them;
//   - the world-set directory (LAB_GAME_WORLD_PATH) supplies every
//     world: identity, generation inputs, resource profile, naming
//     style, lexicon and bestiary, one YAML file per world, each
//     decoded and validated the same way the balance file is.
//
// Configuration is read once, at process start-up, through Load. There is
// no reload path: a changed environment variable or balance file has no
// effect until the process restarts.
package config
