package ingest

import (
	"testing"
	"time"

	"github.com/mymmrac/telego"
)

// TestDerive_perRow builds an Update with exactly one payload field set
// per the kind table's own row, and asserts the derived kind, its date extraction
// (present or absent) and its chat-id extraction (present or absent).
func TestDerive_perRow(t *testing.T) {
	t.Parallel()

	const wantDate = int64(1700000000)
	const wantChatID = int64(-1001234567890)
	msg := func() *telego.Message {
		return &telego.Message{Date: wantDate, Chat: telego.Chat{ID: wantChatID}}
	}
	cm := func() *telego.ChatMemberUpdated {
		return &telego.ChatMemberUpdated{Date: wantDate, Chat: telego.Chat{ID: wantChatID}}
	}

	cases := []struct {
		name     string
		build    func() *telego.Update
		wantKind Kind
		wantDate bool
		wantChat bool
	}{
		{"message", func() *telego.Update { return &telego.Update{Message: msg()} }, KindMessage, true, true},
		{"edited_message", func() *telego.Update { return &telego.Update{EditedMessage: msg()} }, KindEditedMessage, true, true},
		{"channel_post", func() *telego.Update { return &telego.Update{ChannelPost: msg()} }, KindChannelPost, true, true},
		{"edited_channel_post", func() *telego.Update { return &telego.Update{EditedChannelPost: msg()} }, KindEditedChannelPost, true, true},
		{"business_connection", func() *telego.Update {
			return &telego.Update{BusinessConnection: &telego.BusinessConnection{Date: wantDate}}
		}, KindBusinessConnection, true, false},
		{"business_message", func() *telego.Update { return &telego.Update{BusinessMessage: msg()} }, KindBusinessMessage, true, true},
		{"edited_business_message", func() *telego.Update { return &telego.Update{EditedBusinessMessage: msg()} }, KindEditedBusinessMessage, true, true},
		{"deleted_business_messages", func() *telego.Update {
			return &telego.Update{DeletedBusinessMessages: &telego.BusinessMessagesDeleted{Chat: telego.Chat{ID: wantChatID}}}
		}, KindDeletedBusinessMessages, false, true},
		{"guest_message", func() *telego.Update { return &telego.Update{GuestMessage: msg()} }, KindGuestMessage, true, true},
		{"message_reaction", func() *telego.Update {
			return &telego.Update{MessageReaction: &telego.MessageReactionUpdated{Chat: telego.Chat{ID: wantChatID}, Date: wantDate}}
		}, KindMessageReaction, true, true},
		{"message_reaction_count", func() *telego.Update {
			return &telego.Update{MessageReactionCount: &telego.MessageReactionCountUpdated{Chat: telego.Chat{ID: wantChatID}, Date: wantDate}}
		}, KindMessageReactionCount, true, true},
		{"inline_query", func() *telego.Update { return &telego.Update{InlineQuery: &telego.InlineQuery{}} }, KindInlineQuery, false, false},
		{"chosen_inline_result", func() *telego.Update { return &telego.Update{ChosenInlineResult: &telego.ChosenInlineResult{}} }, KindChosenInlineResult, false, false},
		{"callback_query", func() *telego.Update { return &telego.Update{CallbackQuery: &telego.CallbackQuery{ID: "cq1"}} }, KindCallbackQuery, false, false},
		{"shipping_query", func() *telego.Update { return &telego.Update{ShippingQuery: &telego.ShippingQuery{}} }, KindShippingQuery, false, false},
		{"pre_checkout_query", func() *telego.Update { return &telego.Update{PreCheckoutQuery: &telego.PreCheckoutQuery{}} }, KindPreCheckoutQuery, false, false},
		{"purchased_paid_media", func() *telego.Update { return &telego.Update{PurchasedPaidMedia: &telego.PaidMediaPurchased{}} }, KindPurchasedPaidMedia, false, false},
		{"poll", func() *telego.Update { return &telego.Update{Poll: &telego.Poll{}} }, KindPoll, false, false},
		{"poll_answer", func() *telego.Update { return &telego.Update{PollAnswer: &telego.PollAnswer{}} }, KindPollAnswer, false, false},
		{"my_chat_member", func() *telego.Update { return &telego.Update{MyChatMember: cm()} }, KindMyChatMember, true, true},
		{"chat_member", func() *telego.Update { return &telego.Update{ChatMember: cm()} }, KindChatMember, true, true},
		{"chat_join_request", func() *telego.Update {
			return &telego.Update{ChatJoinRequest: &telego.ChatJoinRequest{Chat: telego.Chat{ID: wantChatID}, Date: wantDate}}
		}, KindChatJoinRequest, true, true},
		{"chat_boost", func() *telego.Update {
			return &telego.Update{ChatBoost: &telego.ChatBoostUpdated{Chat: telego.Chat{ID: wantChatID}}}
		}, KindChatBoost, false, true},
		{"removed_chat_boost", func() *telego.Update {
			return &telego.Update{RemovedChatBoost: &telego.ChatBoostRemoved{Chat: telego.Chat{ID: wantChatID}}}
		}, KindRemovedChatBoost, false, true},
		{"managed_bot", func() *telego.Update { return &telego.Update{ManagedBot: &telego.ManagedBotUpdated{}} }, KindManagedBot, false, false},
		{"subscription", func() *telego.Update { return &telego.Update{Subscription: &telego.BotSubscriptionUpdated{}} }, KindSubscription, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			u := tc.build()

			if got := Derive(u); got != tc.wantKind {
				t.Errorf("Derive() = %q, want %q", got, tc.wantKind)
			}

			gotDate, ok := Date(u)
			if ok != tc.wantDate {
				t.Errorf("Date() ok = %v, want %v", ok, tc.wantDate)
			}
			if tc.wantDate && !gotDate.Equal(time.Unix(wantDate, 0).UTC()) {
				t.Errorf("Date() = %v, want %v", gotDate, time.Unix(wantDate, 0).UTC())
			}

			gotChat, ok := ChatID(u)
			if ok != tc.wantChat {
				t.Errorf("ChatID() ok = %v, want %v", ok, tc.wantChat)
			}
			if tc.wantChat && gotChat != wantChatID {
				t.Errorf("ChatID() = %d, want %d", gotChat, wantChatID)
			}
		})
	}
}

// TestDerive_noPayloadFieldSetYieldsZeroKind asserts the unrouted case: a
// telego.Update with no payload field set derives the zero Kind.
func TestDerive_noPayloadFieldSetYieldsZeroKind(t *testing.T) {
	t.Parallel()

	u := &telego.Update{UpdateID: 42}
	if got := Derive(u); got != "" {
		t.Errorf("Derive(empty update) = %q, want the zero Kind", got)
	}
	if _, ok := Date(u); ok {
		t.Error("Date(empty update) reported present, want absent")
	}
	if _, ok := ChatID(u); ok {
		t.Error("ChatID(empty update) reported present, want absent")
	}
}
