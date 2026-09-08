package tg

import "testing"

// TestClassifyMethod exercises the classifier against real method
// names extracted from the pinned telego version's own generated methods,
// including the trap cases: the bare edit*/delete* prefixes would sweep
// in non-message methods, and the prefix form must not do that.
func TestClassifyMethod(t *testing.T) {
	t.Parallel()
	cases := []struct {
		method string
		want   MethodClass
	}{
		// ClassMessage: send*, copyMessage*, forwardMessage*.
		{"sendMessage", ClassMessage},
		{"sendPhoto", ClassMessage},
		{"sendChatAction", ClassMessage},
		{"sendGift", ClassMessage},
		{"sendMessageDraft", ClassMessage},
		{"sendChatJoinRequestWebApp", ClassMessage},
		{"copyMessage", ClassMessage},
		{"copyMessages", ClassMessage},
		{"forwardMessage", ClassMessage},
		{"forwardMessages", ClassMessage},

		// ClassEdit: editMessage*, editEphemeralMessage*, deleteMessage*,
		// deleteEphemeralMessage*, and the two exact names.
		{"editMessageText", ClassEdit},
		{"editMessageCaption", ClassEdit},
		{"editMessageMedia", ClassEdit},
		{"editMessageReplyMarkup", ClassEdit},
		{"editMessageLiveLocation", ClassEdit},
		{"editMessageChecklist", ClassEdit},
		{"editEphemeralMessageCaption", ClassEdit},
		{"editEphemeralMessageMedia", ClassEdit},
		{"editEphemeralMessageReplyMarkup", ClassEdit},
		{"editEphemeralMessageText", ClassEdit},
		{"deleteMessage", ClassEdit},
		{"deleteMessages", ClassEdit},
		{"deleteMessageReaction", ClassEdit},
		{"deleteEphemeralMessage", ClassEdit},
		{"stopMessageLiveLocation", ClassEdit},
		{"stopPoll", ClassEdit},

		// ClassOther: the trap cases — bare edit*/delete* prefixes that
		// are not message operations, and everything else.
		{"editChatInviteLink", ClassOther},
		{"editChatSubscriptionInviteLink", ClassOther},
		{"editForumTopic", ClassOther},
		{"editGeneralForumTopic", ClassOther},
		{"editStory", ClassOther},
		{"editUserStarSubscription", ClassOther},
		{"deleteAllMessageReactions", ClassOther},
		{"deleteBusinessMessages", ClassOther},
		{"deleteChatPhoto", ClassOther},
		{"deleteChatStickerSet", ClassOther},
		{"deleteForumTopic", ClassOther},
		{"deleteMyCommands", ClassOther},
		{"deleteStickerFromSet", ClassOther},
		{"deleteStickerSet", ClassOther},
		{"deleteStory", ClassOther},
		{"deleteWebhook", ClassOther},
		{"getMe", ClassOther},
		{"getUpdates", ClassOther},
		{"answerCallbackQuery", ClassOther},
		{"setMyCommands", ClassOther},
	}
	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			t.Parallel()
			if got := classifyMethod(tc.method); got != tc.want {
				t.Errorf("classifyMethod(%q) = %v, want %v", tc.method, got, tc.want)
			}
		})
	}
}

func TestMethodClass_String(t *testing.T) {
	t.Parallel()
	cases := []struct {
		c    MethodClass
		want string
	}{
		{ClassMessage, "ClassMessage"},
		{ClassEdit, "ClassEdit"},
		{ClassOther, "ClassOther"},
		// Self-review round 5's coverage sweep: the default arm was
		// never exercised. MethodClass is exported and this package
		// adds no value outside 0-2 itself, so an out-of-range value is
		// a genuine (if unusual) caller mistake, not a fabricated
		// scenario — the same fallback ClassOther already answers.
		{MethodClass(99), "ClassOther"},
	}
	for _, tc := range cases {
		if got := tc.c.String(); got != tc.want {
			t.Errorf("%v.String() = %q, want %q", int(tc.c), got, tc.want)
		}
	}
}
