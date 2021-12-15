package note

import (
	"fmt"

	"github.com/goplaid/web"
	"github.com/goplaid/x/i18n"
	"github.com/goplaid/x/presets"
	. "github.com/goplaid/x/vuetify"
	h "github.com/theplant/htmlgo"
	"gorm.io/gorm"
)

type contextUserIDKey int

const (
	UserIDKey contextUserIDKey = iota
	UserKey
)

func getUserData(ctx *web.EventContext) (userID uint, creator string) {
	if ctx.R.Context().Value(UserIDKey) != nil {
		userID = ctx.R.Context().Value(UserIDKey).(uint)
	}
	if ctx.R.Context().Value(UserKey) != nil {
		creator = ctx.R.Context().Value(UserKey).(string)
	}
	return
}

func getTableName(db *gorm.DB, obj interface{}) string {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(obj); err != nil {
		return ""
	}
	return stmt.Schema.Name
}

func getNotesTab(ctx *web.EventContext, db *gorm.DB, resourceType string, resourceId string) h.HTMLComponent {
	msgr := i18n.MustGetModuleMessages(ctx.R, I18nNoteKey, Messages_en_US).(*Messages)

	c := h.Div(
		web.Scope(
			VCardText(
				h.Text(msgr.NewNote),
				VRow(VCol(VTextField().Attr(web.VFieldName("Content")...).Clearable(true))),
			),
			VCardActions(h.Components(
				VSpacer(),
				VBtn(presets.MustGetMessages(ctx.R).Create).
					Color("primary").
					Attr("@click", web.Plaid().
						EventFunc(createNoteEvent).
						Query("resource_id", resourceId).
						Query("resource_type", resourceType).
						Go(),
					),
			)),
		).VSlot("{plaidForm}"),
	)

	var notes []QorNote
	db.Where("resource_type = ? and resource_id = ?", resourceType, resourceId).
		Order("id DESC").Find(&notes)

	for _, note := range notes {
		c.AppendChildren(
			VCard(
				VCardTitle(h.Text(fmt.Sprintf("%v - %v", note.Creator, note.CreatedAt.Format("2006-01-02 15:04:05 MST")))),
				VCardText(h.Text(note.Content)),
			),
		)
	}
	return c
}