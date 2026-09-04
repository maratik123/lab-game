// Package config loads and validates lab-game's runtime configuration from
// three disjoint sources: the process environment (secrets, runtime
// settings, file paths), a balance YAML file (every game constant), and a
// world-set path (existence only — its content is not decoded here).
//
// TODO(#18): this comment is provisional (design D7) — a later subtask in
// this issue rewrites its body to AC13's wording (reload policy + each
// source's exclusive domain).
package config
