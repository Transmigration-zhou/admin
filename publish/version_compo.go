package publish

import (
	"fmt"
	"net/url"
	"reflect"

	h "github.com/theplant/htmlgo"
	"gorm.io/gorm"

	"github.com/qor5/admin/v3/activity"
	"github.com/qor5/admin/v3/note"
	"github.com/qor5/admin/v3/presets"
	"github.com/qor5/admin/v3/presets/actions"
	"github.com/qor5/admin/v3/utils"
	v "github.com/qor5/ui/v3/vuetify"
	vx "github.com/qor5/ui/v3/vuetifyx"
	"github.com/qor5/web/v3"
	"github.com/qor5/x/v3/i18n"
)

type VersionComponentConfig struct {
	// If you want to use custom publish dialog, you can update the portal named PublishCustomDialogPortalName
	PublishEvent   func(obj interface{}, field *presets.FieldContext, ctx *web.EventContext) string
	UnPublishEvent func(obj interface{}, field *presets.FieldContext, ctx *web.EventContext) string
	RePublishEvent func(obj interface{}, field *presets.FieldContext, ctx *web.EventContext) string
	Top            bool
}

func DefaultVersionComponentFunc(b *presets.ModelBuilder, cfg ...VersionComponentConfig) presets.FieldComponentFunc {
	var config VersionComponentConfig
	if len(cfg) > 0 {
		config = cfg[0]
	}
	return func(obj interface{}, field *presets.FieldContext, ctx *web.EventContext) h.HTMLComponent {
		var (
			version        VersionInterface
			status         StatusInterface
			primarySlugger presets.SlugEncoder
			ok             bool
			versionSwitch  *v.VChipBuilder
			publishBtn     h.HTMLComponent
		)
		msgr := i18n.MustGetModuleMessages(ctx.R, I18nPublishKey, Messages_en_US).(*Messages)
		utilsMsgr := i18n.MustGetModuleMessages(ctx.R, utils.I18nUtilsKey, utils.Messages_en_US).(*utils.Messages)

		primarySlugger, ok = obj.(presets.SlugEncoder)
		if !ok {
			panic("obj should be SlugEncoder")
		}

		div := h.Div().Class("w-100 d-inline-flex")

		if version, ok = obj.(VersionInterface); ok {
			versionSwitch = v.VChip(
				h.Text(version.GetVersionName()),
			).Label(true).Variant(v.VariantOutlined).
				Attr("style", "height:40px;").
				Attr("@click", web.Plaid().EventFunc(actions.OpenListingDialog).
					URL(b.Info().PresetsPrefix()+"/"+field.ModelInfo.URIName()+"-version-list-dialog").
					Query("select_id", primarySlugger.PrimarySlug()).
					Go()).
				Class(v.W100)
			if status, ok = obj.(StatusInterface); ok {
				versionSwitch.AppendChildren(v.VChip(h.Text(GetStatusText(status.GetStatus(), msgr))).Label(true).Color(GetStatusColor(status.GetStatus())).Size(v.SizeSmall).Class("px-1 mx-1 ml-2"))
			}
			versionSwitch.AppendIcon("mdi-chevron-down")

			div.AppendChildren(versionSwitch)
			div.AppendChildren(v.VBtn(msgr.Duplicate).PrependIcon("mdi-file-document-multiple").
				Height(40).Class("ml-2").Variant(v.VariantOutlined).
				Attr("@click", fmt.Sprintf(`locals.action="%s";locals.commonConfirmDialog = true`, EventDuplicateVersion)))
		}

		if status, ok = obj.(StatusInterface); ok {
			switch status.GetStatus() {
			case StatusDraft, StatusOffline:
				publishEvent := fmt.Sprintf(`locals.action="%s";locals.commonConfirmDialog = true`, EventPublish)
				if config.PublishEvent != nil {
					publishEvent = config.PublishEvent(obj, field, ctx)
				}
				publishBtn = h.Div(
					v.VBtn(msgr.Publish).Attr("@click", publishEvent).Rounded("0").
						Class("rounded-s ml-2").Variant(v.VariantFlat).Color(v.ColorPrimary).Height(40),
				)
			case StatusOnline:
				unPublishEvent := fmt.Sprintf(`locals.action="%s";locals.commonConfirmDialog = true`, EventUnpublish)
				if config.UnPublishEvent != nil {
					unPublishEvent = config.UnPublishEvent(obj, field, ctx)
				}
				rePublishEvent := fmt.Sprintf(`locals.action="%s";locals.commonConfirmDialog = true`, EventRepublish)
				if config.RePublishEvent != nil {
					rePublishEvent = config.RePublishEvent(obj, field, ctx)
				}
				publishBtn = h.Div(
					v.VBtn(msgr.Unpublish).Attr("@click", unPublishEvent).
						Class("ml-2").Variant(v.VariantFlat).Color(v.ColorError).Height(40),
					v.VBtn(msgr.Republish).Attr("@click", rePublishEvent).
						Class("ml-2").Variant(v.VariantFlat).Color(v.ColorPrimary).Height(40),
				).Class("d-inline-flex")
			}
			div.AppendChildren(publishBtn)
			// Publish/Unpublish/Republish ConfirmDialog
			div.AppendChildren(
				utils.ConfirmDialog(msgr.Areyousure, web.Plaid().EventFunc(web.Var("locals.action")).
					Query(presets.ParamID, primarySlugger.PrimarySlug()).Go(),
					utilsMsgr),
			)
			// Publish/Unpublish/Republish CustomDialog
			if config.UnPublishEvent != nil || config.RePublishEvent != nil || config.PublishEvent != nil {
				div.AppendChildren(web.Portal().Name(PortalPublishCustomDialog))
			}
		}

		if _, ok = obj.(ScheduleInterface); ok && status.GetStatus() == StatusDraft {
			var scheduleBtn h.HTMLComponent
			clickEvent := web.POST().
				EventFunc(eventSchedulePublishDialog).
				Query(presets.ParamOverlay, actions.Dialog).
				Query(presets.ParamID, primarySlugger.PrimarySlug()).
				URL(fmt.Sprintf("%s/%s-version-list-dialog", b.Info().PresetsPrefix(), b.Info().URIName())).Go()
			if config.Top {
				scheduleBtn = v.VAutocomplete().PrependInnerIcon("mdi-alarm").Density(v.DensityCompact).
					Variant(v.FieldVariantSoloFilled).ModelValue("Schedule Publish Time").
					BgColor(v.ColorPrimaryLighten2).Readonly(true).
					Width(600).HideDetails(true).Attr("@click", clickEvent).Class("ml-2 text-caption")
			} else {
				scheduleBtn = v.VBtn("").Children(v.VIcon("mdi-alarm").Size(v.SizeXLarge)).Rounded("0").Class("ml-1 rounded-e").
					Variant(v.VariantFlat).Color(v.ColorPrimary).Height(40).Attr("@click", clickEvent)
			}
			div.AppendChildren(scheduleBtn)
			// SchedulePublishDialog
			div.AppendChildren(web.Portal().Name(PortalSchedulePublishDialog))
		}

		return web.Scope(div).VSlot(" { locals } ").Init(fmt.Sprintf(`{action: "", commonConfirmDialog: false }`))
	}
}

func DefaultVersionBar(db *gorm.DB) presets.ObjectComponentFunc {
	return func(obj interface{}, ctx *web.EventContext) h.HTMLComponent {
		msgr := i18n.MustGetModuleMessages(ctx.R, I18nPublishKey, Messages_en_US).(*Messages)
		res := h.Div().Class("d-inline-flex align-center")

		slugEncoderIf := obj.(presets.SlugEncoder)
		slugDncoderIf := obj.(presets.SlugDecoder)
		mp := slugDncoderIf.PrimaryColumnValuesBySlug(slugEncoderIf.PrimarySlug())

		currentObj := reflect.New(reflect.TypeOf(obj).Elem()).Interface()
		err := db.Where("id = ?", mp["id"]).Where("status = ?", StatusOnline).First(&currentObj).Error
		if err != nil {
			return res
		}
		versionIf := currentObj.(VersionInterface)
		currentVersionStr := fmt.Sprintf("%s: %s", msgr.OnlineVersion, versionIf.GetVersionName())
		res.AppendChildren(v.VChip(h.Span(currentVersionStr)).Density(v.DensityCompact).Color(v.ColorSuccess))

		if _, ok := currentObj.(ScheduleInterface); !ok {
			return res
		}

		nextObj := reflect.New(reflect.TypeOf(obj).Elem()).Interface()
		flagTime := db.NowFunc()
		count := int64(0)
		err = db.Model(nextObj).Where("id = ?", mp["id"]).Where("scheduled_start_at >= ?", flagTime).Count(&count).Error
		if err != nil {
			return res
		}

		if count == 0 {
			return res
		}

		err = db.Where("id = ?", mp["id"]).Where("scheduled_start_at >= ?", flagTime).Order("scheduled_start_at ASC").First(&nextObj).Error
		if err != nil {
			return res
		}
		res.AppendChildren(
			h.Div(
				h.Div().Class(fmt.Sprintf(`w-100 bg-%s`, v.ColorSuccessLighten2)).Style("height:4px"),
				v.VIcon("mdi-circle").Size(v.SizeXSmall).Color(v.ColorSuccess).Attr("style", "position:absolute;left:0;right:0;margin-left:auto;margin-right:auto"),
			).Class("h-100 d-flex align-center").Style("position:relative;width:40px"),
		)
		versionIf = nextObj.(VersionInterface)
		// TODO use nextVersion I18n
		nextText := fmt.Sprintf("%s: %s", msgr.OnlineVersion, versionIf.GetVersionName())
		res.AppendChildren(v.VChip(h.Span(nextText)).Density(v.DensityCompact).Color(v.ColorSecondary))
		if count >= 2 {
			res.AppendChildren(
				h.Div(
					h.Div().Class(fmt.Sprintf(`w-100 bg-%s`, v.ColorSecondaryLighten1)).Style("height:4px"),
				).Class("h-100 d-flex align-center").Style("width:40px"),
				h.Div(
					h.Text(fmt.Sprintf(`+%v`, count)),
				).Class(fmt.Sprintf(`text-caption bg-%s`, v.ColorSecondaryLighten1)),
			)
		}
		return res
	}
}

func configureVersionListDialog(db *gorm.DB, b *presets.Builder, pm *presets.ModelBuilder, publisher *Builder, ab *activity.Builder) {
	// actually, VersionListDialog is a listing
	// use this URL : URLName-version-list-dialog
	mb := b.Model(pm.NewModel()).
		URIName(pm.Info().URIName() + "-version-list-dialog").
		InMenu(false)

	registerEventFuncs(db, mb, publisher, ab)

	searcher := mb.Listing().Searcher
	lb := mb.Listing("Version", "State", "StartAt", "EndAt", "Notes", "Option").
		SearchColumns("version", "version_name").
		PerPage(10).
		SearchFunc(func(model interface{}, params *presets.SearchParams, ctx *web.EventContext) (r interface{}, totalCount int, err error) {
			id := ctx.R.FormValue("select_id")
			if id == "" {
				id = ctx.R.FormValue("f_select_id")
			}
			if id != "" {
				cs := mb.NewModel().(presets.SlugDecoder).PrimaryColumnValuesBySlug(id)
				con := presets.SQLCondition{
					Query: "id = ?",
					Args:  []interface{}{cs["id"]},
				}
				params.SQLConditions = append(params.SQLConditions, &con)
			}
			params.OrderBy = "created_at DESC"

			return searcher(model, params, ctx)
		})
	lb.CellWrapperFunc(func(cell h.MutableAttrHTMLComponent, id string, obj interface{}, dataTableID string) h.HTMLComponent {
		return cell
	})
	lb.Field("Version").ComponentFunc(func(obj interface{}, field *presets.FieldContext, ctx *web.EventContext) h.HTMLComponent {
		versionName := obj.(VersionInterface).GetVersionName()
		p := obj.(presets.SlugEncoder)
		id := ctx.R.FormValue("select_id")
		if id == "" {
			id = ctx.R.FormValue("f_select_id")
		}

		queries := ctx.Queries()
		queries.Set("select_id", p.PrimarySlug())
		onChange := web.Plaid().URL(ctx.R.URL.Path).Queries(queries).EventFunc(actions.UpdateListingDialog).Go()

		return h.Td().Children(
			h.Div().Class("d-inline-flex align-center").Children(
				v.VRadio().ModelValue(p.PrimarySlug()).TrueValue(id).Attr("@change", onChange),
				h.Text(versionName),
			),
		)
	})
	lb.Field("State").ComponentFunc(StatusListFunc())
	lb.Field("StartAt").ComponentFunc(func(obj interface{}, field *presets.FieldContext, ctx *web.EventContext) h.HTMLComponent {
		p := obj.(ScheduleInterface)
		var showTime string
		if p.GetScheduledStartAt() != nil {
			showTime = p.GetScheduledStartAt().Format("2006-01-02 15:04")
		}

		return h.Td(
			h.Text(showTime),
		)
	}).Label("Start at")
	lb.Field("EndAt").ComponentFunc(func(obj interface{}, field *presets.FieldContext, ctx *web.EventContext) h.HTMLComponent {
		p := obj.(ScheduleInterface)
		var showTime string
		if p.GetScheduledEndAt() != nil {
			showTime = p.GetScheduledEndAt().Format("2006-01-02 15:04")
		}
		return h.Td(
			h.Text(showTime),
		)
	}).Label("End at")

	lb.Field("Notes").ComponentFunc(func(obj interface{}, field *presets.FieldContext, ctx *web.EventContext) h.HTMLComponent {
		p := obj.(presets.SlugEncoder)
		rt := pm.Info().Label()
		ri := p.PrimarySlug()
		userID, _ := note.GetUserData(ctx)
		count := note.GetUnreadNotesCount(db, userID, rt, ri)

		return h.Td(
			h.If(count > 0,
				v.VBadge().Content(count).Color("red"),
			).Else(
				h.Text(""),
			),
		)
	}).Label("Unread Notes")

	lb.Field("Option").ComponentFunc(func(obj interface{}, field *presets.FieldContext, ctx *web.EventContext) h.HTMLComponent {
		versionIf := obj.(VersionInterface)
		statusIf := obj.(StatusInterface)
		id := ctx.R.FormValue("select_id")
		if id == "" {
			id = ctx.R.FormValue("f_select_id")
		}
		versionName := versionIf.GetVersionName()
		var disable bool
		if statusIf.GetStatus() == StatusOnline || statusIf.GetStatus() == StatusOffline {
			disable = true
		}

		return h.Td().Children(
			// TODO: i18n
			v.VBtn("Rename").Disabled(disable).PrependIcon("mdi-rename-box").Size(v.SizeXSmall).Color(v.ColorPrimary).Variant(v.VariantText).
				Attr("@click", web.Plaid().
					URL(pm.Info().PresetsPrefix()+"/"+mb.Info().URIName()).
					EventFunc(eventRenameVersionDialog).
					Queries(ctx.Queries()).
					Query(presets.ParamOverlay, actions.Dialog).
					Query("rename_id", obj.(presets.SlugEncoder).PrimarySlug()).
					Query("version_name", versionName).
					Go(),
				),
			v.VBtn("Delete").Disabled(disable).PrependIcon("mdi-delete").Size(v.SizeXSmall).Color(v.ColorPrimary).Variant(v.VariantText).
				Attr("@click", web.Plaid().
					URL(pm.Info().PresetsPrefix()+"/"+mb.Info().URIName()).
					EventFunc(eventDeleteVersionDialog).
					Queries(ctx.Queries()).
					Query(presets.ParamOverlay, actions.Dialog).
					Query("delete_id", obj.(presets.SlugEncoder).PrimarySlug()).
					Query("version_name", versionName).
					Go(),
				),
		)
	})
	lb.NewButtonFunc(func(ctx *web.EventContext) h.HTMLComponent { return nil })
	lb.FooterAction("Cancel").ButtonCompFunc(func(ctx *web.EventContext) h.HTMLComponent {
		return v.VBtn("Cancel").Variant(v.VariantElevated).Attr("@click", "vars.presetsListingDialog=false")
	})
	lb.FooterAction("Save").ButtonCompFunc(func(ctx *web.EventContext) h.HTMLComponent {
		id := ctx.R.FormValue("select_id")
		if id == "" {
			id = ctx.R.FormValue("f_select_id")
		}

		return v.VBtn("Save").Variant(v.VariantElevated).Color(v.ColorSecondary).Attr("@click", web.Plaid().
			Query("select_id", id).
			URL(pm.Info().PresetsPrefix()+"/"+mb.Info().URIName()).
			EventFunc(eventSelectVersion).
			Go())
	})
	lb.RowMenu().Empty()

	mb.RegisterEventFunc(eventSelectVersion, func(ctx *web.EventContext) (r web.EventResponse, err error) {
		refer, _ := url.Parse(ctx.R.Referer())
		newQueries := refer.Query()
		id := ctx.R.FormValue("select_id")

		if !pm.HasDetailing() {
			// close dialog and open editing
			newQueries.Add(presets.ParamID, id)
			r.RunScript = presets.CloseListingDialogVarScript + ";" +
				web.Plaid().EventFunc(actions.Edit).Queries(newQueries).Go()
			return
		}
		if !pm.Detailing().GetDrawer() {
			// open detailing without drawer
			// jump URL to support referer
			r.PushState = web.Location(newQueries).URL(pm.Info().DetailingHref(id))
			return
		}
		newQueries.Add(presets.ParamID, id)
		// close dialog and open detailingDrawer
		r.RunScript = presets.CloseListingDialogVarScript + ";" +
			web.Plaid().EventFunc(actions.DetailingDrawer).Queries(newQueries).Go()
		return
	})

	lb.FilterDataFunc(func(ctx *web.EventContext) vx.FilterData {
		return []*vx.FilterItem{
			{
				Key:          "all",
				Invisible:    true,
				SQLCondition: ``,
			},
			{
				Key:          "online_version",
				Invisible:    true,
				SQLCondition: `status = 'online'`,
			},
			{
				Key:          "named_versions",
				Invisible:    true,
				SQLCondition: `version <> version_name`,
			},
		}
	})

	lb.FilterTabsFunc(func(ctx *web.EventContext) []*presets.FilterTab {
		msgr := i18n.MustGetModuleMessages(ctx.R, I18nPublishKey, Messages_en_US).(*Messages)
		id := ctx.R.FormValue("select_id")
		if id == "" {
			id = ctx.R.FormValue("f_select_id")
		}
		return []*presets.FilterTab{
			{
				Label: msgr.FilterTabAllVersions,
				ID:    "all",
				Query: url.Values{"all": []string{"1"}, "select_id": []string{id}},
			},
			{
				Label: msgr.FilterTabOnlineVersion,
				ID:    "online_version",
				Query: url.Values{"online_version": []string{"1"}, "select_id": []string{id}},
			},
			{
				Label: msgr.FilterTabNamedVersions,
				ID:    "named_versions",
				Query: url.Values{"named_versions": []string{"1"}, "select_id": []string{id}},
			},
		}
	})
}
