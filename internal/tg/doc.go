// Package tg is the one client every outbound Telegram Bot API call in
// this project goes through. It is a thin layer whose entire policy —
// the retry loop, the retry_after wait, the global and per-chat rate
// limiters, the outbound gate seam, and the per-call observation point —
// lives inside one implementation of telego's own telegoapi.Caller
// interface. telego's low-level, generated API is used for
// complete Bot API coverage by construction; telego's helper layer
// (retries, idempotency, rate limiting) is never used — those are this
// package's job.
//
// Every outbound-call method is telego's own generated method on
// *telego.Bot, obtained through Client.API(); every one of them is routed
// through this package's caller with no way to route around it for the
// generated-method surface.
package tg
