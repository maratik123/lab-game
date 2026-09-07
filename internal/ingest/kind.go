package ingest

import (
	"time"

	"github.com/mymmrac/telego"
)

// Kind identifies one Bot API update type — the exact tokens
// GetUpdatesParams.AllowedUpdates and telego's own "Update types you want
// your bot to receive" constants use (design D4). The zero Kind ("") is
// never a real update type: it is what Derive returns for an update
// matching no table row (unrouted, AC9).
type Kind string

// The Bot API's update types, mirroring telego's own constant block
// exactly (methods.go's MessageUpdates .. ManagedBot, plus Subscription —
// design D4). A drift check in guards_test.go (subtask 11) reflects over
// telego.Update's exported pointer fields and asserts each one's json tag
// is either a table row here or a named exemption.
const (
	KindMessage                 Kind = "message"
	KindEditedMessage           Kind = "edited_message"
	KindChannelPost             Kind = "channel_post"
	KindEditedChannelPost       Kind = "edited_channel_post"
	KindBusinessConnection      Kind = "business_connection"
	KindBusinessMessage         Kind = "business_message"
	KindEditedBusinessMessage   Kind = "edited_business_message"
	KindDeletedBusinessMessages Kind = "deleted_business_messages"
	KindGuestMessage            Kind = "guest_message"
	KindMessageReaction         Kind = "message_reaction"
	KindMessageReactionCount    Kind = "message_reaction_count"
	KindInlineQuery             Kind = "inline_query"
	KindChosenInlineResult      Kind = "chosen_inline_result"
	KindCallbackQuery           Kind = "callback_query"
	KindShippingQuery           Kind = "shipping_query"
	KindPreCheckoutQuery        Kind = "pre_checkout_query"
	KindPurchasedPaidMedia      Kind = "purchased_paid_media"
	KindPoll                    Kind = "poll"
	KindPollAnswer              Kind = "poll_answer"
	KindMyChatMember            Kind = "my_chat_member"
	KindChatMember              Kind = "chat_member"
	KindChatJoinRequest         Kind = "chat_join_request"
	KindChatBoost               Kind = "chat_boost"
	KindRemovedChatBoost        Kind = "removed_chat_boost"
	KindManagedBot              Kind = "managed_bot"
	KindSubscription            Kind = "subscription"
)

// kindRow is one table row: the kind, a probe reporting whether u carries
// that payload, and optional extractors for the update's own date and
// its destination chat id — supplied per PAYLOAD TYPE, not per kind, so
// every *telego.Message kind shares messageDate/messageChatID and both
// *telego.ChatMemberUpdated kinds share chatMemberDate/chatMemberChatID
// (design D4). date and chatID are nil for a kind whose payload declares
// neither.
type kindRow struct {
	kind    Kind
	present func(*telego.Update) bool
	date    func(*telego.Update) (time.Time, bool)
	chatID  func(*telego.Update) (int64, bool)
}

// messageDate and messageChatID extract telego.Message's own Date and
// Chat.ID fields — shared by every *telego.Message-payload kind (design
// D13).
func messageDate(m *telego.Message) (time.Time, bool) {
	return time.Unix(m.Date, 0).UTC(), true
}

func messageChatID(m *telego.Message) (int64, bool) {
	return m.Chat.ID, true
}

// chatMemberDate and chatMemberChatID extract telego.ChatMemberUpdated's
// own Date and Chat.ID fields — shared by KindMyChatMember and
// KindChatMember (design D13).
func chatMemberDate(c *telego.ChatMemberUpdated) (time.Time, bool) {
	return time.Unix(c.Date, 0).UTC(), true
}

func chatMemberChatID(c *telego.ChatMemberUpdated) (int64, bool) {
	return c.Chat.ID, true
}

// businessConnectionDate extracts telego.BusinessConnection's own Date
// field — KindBusinessConnection's payload declares a Date but no Chat
// (design D4/D13).
func businessConnectionDate(b *telego.BusinessConnection) (time.Time, bool) {
	return time.Unix(b.Date, 0).UTC(), true
}

// businessMessagesDeletedChatID extracts telego.BusinessMessagesDeleted's
// own Chat.ID field — KindDeletedBusinessMessages's payload declares a
// Chat but no Date (design D4/D13).
func businessMessagesDeletedChatID(b *telego.BusinessMessagesDeleted) (int64, bool) {
	return b.Chat.ID, true
}

// messageReactionDate and messageReactionChatID extract
// telego.MessageReactionUpdated's own Date and Chat.ID fields —
// KindMessageReaction's payload declares both (design D4/D13).
func messageReactionDate(m *telego.MessageReactionUpdated) (time.Time, bool) {
	return time.Unix(m.Date, 0).UTC(), true
}

func messageReactionChatID(m *telego.MessageReactionUpdated) (int64, bool) {
	return m.Chat.ID, true
}

// messageReactionCountDate and messageReactionCountChatID extract
// telego.MessageReactionCountUpdated's own Date and Chat.ID fields —
// KindMessageReactionCount's payload declares both (design D4/D13).
func messageReactionCountDate(m *telego.MessageReactionCountUpdated) (time.Time, bool) {
	return time.Unix(m.Date, 0).UTC(), true
}

func messageReactionCountChatID(m *telego.MessageReactionCountUpdated) (int64, bool) {
	return m.Chat.ID, true
}

// chatJoinRequestDate and chatJoinRequestChatID extract
// telego.ChatJoinRequest's own Date and Chat.ID fields —
// KindChatJoinRequest's payload declares both (design D4/D13).
func chatJoinRequestDate(c *telego.ChatJoinRequest) (time.Time, bool) {
	return time.Unix(c.Date, 0).UTC(), true
}

func chatJoinRequestChatID(c *telego.ChatJoinRequest) (int64, bool) {
	return c.Chat.ID, true
}

// chatBoostUpdatedChatID extracts telego.ChatBoostUpdated's own Chat.ID
// field — KindChatBoost's payload declares a Chat but no Date (design
// D4/D13).
func chatBoostUpdatedChatID(c *telego.ChatBoostUpdated) (int64, bool) {
	return c.Chat.ID, true
}

// chatBoostRemovedChatID extracts telego.ChatBoostRemoved's own Chat.ID
// field — KindRemovedChatBoost's payload declares a Chat but no Date
// (RemoveDate is a distinct field, not the update's own Date — design
// D4/D13).
func chatBoostRemovedChatID(c *telego.ChatBoostRemoved) (int64, bool) {
	return c.Chat.ID, true
}

// kindTable is the one explicit table D4 requires: production code, not
// reflection. Every entry's present probe checks exactly the payload
// field the row's json tag names.
var kindTable = []kindRow{
	{
		kind:    KindMessage,
		present: func(u *telego.Update) bool { return u.Message != nil },
		date:    func(u *telego.Update) (time.Time, bool) { return messageDate(u.Message) },
		chatID:  func(u *telego.Update) (int64, bool) { return messageChatID(u.Message) },
	},
	{
		kind:    KindEditedMessage,
		present: func(u *telego.Update) bool { return u.EditedMessage != nil },
		date:    func(u *telego.Update) (time.Time, bool) { return messageDate(u.EditedMessage) },
		chatID:  func(u *telego.Update) (int64, bool) { return messageChatID(u.EditedMessage) },
	},
	{
		kind:    KindChannelPost,
		present: func(u *telego.Update) bool { return u.ChannelPost != nil },
		date:    func(u *telego.Update) (time.Time, bool) { return messageDate(u.ChannelPost) },
		chatID:  func(u *telego.Update) (int64, bool) { return messageChatID(u.ChannelPost) },
	},
	{
		kind:    KindEditedChannelPost,
		present: func(u *telego.Update) bool { return u.EditedChannelPost != nil },
		date:    func(u *telego.Update) (time.Time, bool) { return messageDate(u.EditedChannelPost) },
		chatID:  func(u *telego.Update) (int64, bool) { return messageChatID(u.EditedChannelPost) },
	},
	{
		kind:    KindBusinessConnection,
		present: func(u *telego.Update) bool { return u.BusinessConnection != nil },
		date:    func(u *telego.Update) (time.Time, bool) { return businessConnectionDate(u.BusinessConnection) },
	},
	{
		kind:    KindBusinessMessage,
		present: func(u *telego.Update) bool { return u.BusinessMessage != nil },
		date:    func(u *telego.Update) (time.Time, bool) { return messageDate(u.BusinessMessage) },
		chatID:  func(u *telego.Update) (int64, bool) { return messageChatID(u.BusinessMessage) },
	},
	{
		kind:    KindEditedBusinessMessage,
		present: func(u *telego.Update) bool { return u.EditedBusinessMessage != nil },
		date:    func(u *telego.Update) (time.Time, bool) { return messageDate(u.EditedBusinessMessage) },
		chatID:  func(u *telego.Update) (int64, bool) { return messageChatID(u.EditedBusinessMessage) },
	},
	{
		kind:    KindDeletedBusinessMessages,
		present: func(u *telego.Update) bool { return u.DeletedBusinessMessages != nil },
		chatID:  func(u *telego.Update) (int64, bool) { return businessMessagesDeletedChatID(u.DeletedBusinessMessages) },
	},
	{
		kind:    KindGuestMessage,
		present: func(u *telego.Update) bool { return u.GuestMessage != nil },
		date:    func(u *telego.Update) (time.Time, bool) { return messageDate(u.GuestMessage) },
		chatID:  func(u *telego.Update) (int64, bool) { return messageChatID(u.GuestMessage) },
	},
	{
		kind:    KindMessageReaction,
		present: func(u *telego.Update) bool { return u.MessageReaction != nil },
		date:    func(u *telego.Update) (time.Time, bool) { return messageReactionDate(u.MessageReaction) },
		chatID:  func(u *telego.Update) (int64, bool) { return messageReactionChatID(u.MessageReaction) },
	},
	{
		kind:    KindMessageReactionCount,
		present: func(u *telego.Update) bool { return u.MessageReactionCount != nil },
		date:    func(u *telego.Update) (time.Time, bool) { return messageReactionCountDate(u.MessageReactionCount) },
		chatID:  func(u *telego.Update) (int64, bool) { return messageReactionCountChatID(u.MessageReactionCount) },
	},
	{
		kind:    KindInlineQuery,
		present: func(u *telego.Update) bool { return u.InlineQuery != nil },
	},
	{
		kind:    KindChosenInlineResult,
		present: func(u *telego.Update) bool { return u.ChosenInlineResult != nil },
	},
	{
		kind:    KindCallbackQuery,
		present: func(u *telego.Update) bool { return u.CallbackQuery != nil },
		// CallbackQuery declares no date field at all (design D13) — date
		// stays nil.
	},
	{
		kind:    KindShippingQuery,
		present: func(u *telego.Update) bool { return u.ShippingQuery != nil },
	},
	{
		kind:    KindPreCheckoutQuery,
		present: func(u *telego.Update) bool { return u.PreCheckoutQuery != nil },
	},
	{
		kind:    KindPurchasedPaidMedia,
		present: func(u *telego.Update) bool { return u.PurchasedPaidMedia != nil },
	},
	{
		kind:    KindPoll,
		present: func(u *telego.Update) bool { return u.Poll != nil },
	},
	{
		kind:    KindPollAnswer,
		present: func(u *telego.Update) bool { return u.PollAnswer != nil },
	},
	{
		kind:    KindMyChatMember,
		present: func(u *telego.Update) bool { return u.MyChatMember != nil },
		date:    func(u *telego.Update) (time.Time, bool) { return chatMemberDate(u.MyChatMember) },
		chatID:  func(u *telego.Update) (int64, bool) { return chatMemberChatID(u.MyChatMember) },
	},
	{
		kind:    KindChatMember,
		present: func(u *telego.Update) bool { return u.ChatMember != nil },
		date:    func(u *telego.Update) (time.Time, bool) { return chatMemberDate(u.ChatMember) },
		chatID:  func(u *telego.Update) (int64, bool) { return chatMemberChatID(u.ChatMember) },
	},
	{
		kind:    KindChatJoinRequest,
		present: func(u *telego.Update) bool { return u.ChatJoinRequest != nil },
		date:    func(u *telego.Update) (time.Time, bool) { return chatJoinRequestDate(u.ChatJoinRequest) },
		chatID:  func(u *telego.Update) (int64, bool) { return chatJoinRequestChatID(u.ChatJoinRequest) },
	},
	{
		kind:    KindChatBoost,
		present: func(u *telego.Update) bool { return u.ChatBoost != nil },
		chatID:  func(u *telego.Update) (int64, bool) { return chatBoostUpdatedChatID(u.ChatBoost) },
	},
	{
		kind:    KindRemovedChatBoost,
		present: func(u *telego.Update) bool { return u.RemovedChatBoost != nil },
		chatID:  func(u *telego.Update) (int64, bool) { return chatBoostRemovedChatID(u.RemovedChatBoost) },
	},
	{
		kind:    KindManagedBot,
		present: func(u *telego.Update) bool { return u.ManagedBot != nil },
	},
	{
		kind:    KindSubscription,
		present: func(u *telego.Update) bool { return u.Subscription != nil },
	},
}

// rowForKind returns kindTable's row for k, and whether one exists.
func rowForKind(k Kind) (kindRow, bool) {
	for _, row := range kindTable {
		if row.kind == k {
			return row, true
		}
	}
	return kindRow{}, false
}

// knownKind reports whether k names a row of kindTable — the predicate
// NewRouter refuses a route without (design D4).
func knownKind(k Kind) bool {
	_, ok := rowForKind(k)
	return ok
}

// Derive reports u's Kind: the first kindTable row whose present probe
// matches, since exactly one of telego.Update's optional payload fields
// is ever set on a real update. An update matching no row (a Bot API
// update type this table has not yet learned, or a hand-built Update
// with nothing set) derives the zero Kind — unrouted, never a panic
// (design D4, AC9).
func Derive(u *telego.Update) Kind {
	for _, row := range kindTable {
		if row.present(u) {
			return row.kind
		}
	}
	return ""
}

// Date reports u's own date, when its derived kind's payload declares
// one (design D13). The boolean is false — never a zero time.Time — for
// a kind whose payload declares no date, because a zero lag would be
// indistinguishable from a healthy poll.
func Date(u *telego.Update) (time.Time, bool) {
	row, ok := rowForKind(Derive(u))
	if !ok || row.date == nil {
		return time.Time{}, false
	}
	return row.date(u)
}

// ChatID reports u's destination chat id, when its derived kind's
// payload declares one (design D4).
func ChatID(u *telego.Update) (int64, bool) {
	row, ok := rowForKind(Derive(u))
	if !ok || row.chatID == nil {
		return 0, false
	}
	return row.chatID(u)
}
