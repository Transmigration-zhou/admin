package publish

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/iancoleman/strcase"
	"github.com/qor/oss"
	"github.com/qor5/admin/v3/activity"
	"github.com/qor5/admin/v3/presets"
	"github.com/qor5/admin/v3/presets/actions"
	"github.com/qor5/admin/v3/utils"
	"github.com/qor5/ui/v3/vuetify"
	"github.com/qor5/web/v3"
	"github.com/sunfmin/reflectutils"
	"github.com/theplant/htmlgo"
	"golang.org/x/text/language"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

type Builder struct {
	db      *gorm.DB
	storage oss.StorageInterface
	// models            []*presets.ModelBuilder
	ab                *activity.Builder
	ctxValueProviders []ContextValueFunc
	afterInstallFuncs []func()
}

type ContextValueFunc func(ctx context.Context) context.Context

func New(db *gorm.DB, storage oss.StorageInterface) *Builder {
	return &Builder{
		db:      db,
		storage: storage,
	}
}

func (b *Builder) Activity(v *activity.Builder) (r *Builder) {
	b.ab = v
	return b
}

func (b *Builder) AfterInstall(f func()) *Builder {
	b.afterInstallFuncs = append(b.afterInstallFuncs, f)
	return b
}

func (b *Builder) ModelInstall(pb *presets.Builder, m *presets.ModelBuilder) error {
	db := b.db
	ab := b.ab

	obj := m.NewModel()
	_ = obj.(presets.SlugEncoder)
	_ = obj.(presets.SlugDecoder)

	if model, ok := obj.(VersionInterface); ok {
		if schedulePublishModel, ok := model.(ScheduleInterface); ok {
			VersionPublishModels[m.Info().URIName()] = reflect.ValueOf(schedulePublishModel).Elem().Interface()
		}

		b.configVersionAndPublish(pb, m, db)
	} else {
		if schedulePublishModel, ok := obj.(ScheduleInterface); ok {
			NonVersionPublishModels[m.Info().URIName()] = reflect.ValueOf(schedulePublishModel).Elem().Interface()
		}
	}

	if model, ok := obj.(ListInterface); ok {
		if schedulePublishModel, ok := model.(ScheduleInterface); ok {
			ListPublishModels[m.Info().URIName()] = reflect.ValueOf(schedulePublishModel).Elem().Interface()
		}
	}

	registerEventFuncsForResource(db, m, b, ab)
	return nil
}

func (b *Builder) configVersionAndPublish(pb *presets.Builder, m *presets.ModelBuilder, db *gorm.DB) {
	ed := m.Editing()
	creating := ed.Creating().Except(VersionsPublishBar)
	var detailing *presets.DetailingBuilder
	if !m.HasDetailing() {
		detailing = m.Detailing().Drawer(true)
	} else {
		detailing = m.Detailing()
	}

	fb := detailing.GetField(VersionsPublishBar)
	if fb != nil && fb.GetCompFunc() == nil {
		fb.ComponentFunc(DefaultVersionComponentFunc(m))
	}

	m.Listing().WrapSearchFunc(makeSearchFunc(db))

	// listing-delete deletes all versions
	{
		// rewrite Delete row menu item to show correct id in prompt message
		m.Listing().RowMenu().RowMenuItem("Delete").ComponentFunc(func(obj interface{}, id string, ctx *web.EventContext) htmlgo.HTMLComponent {
			msgr := presets.MustGetMessages(ctx.R)
			if m.Info().Verifier().Do(presets.PermDelete).ObjectOn(obj).WithReq(ctx.R).IsAllowed() != nil {
				return nil
			}

			promptID := id
			if slugger, ok := obj.(presets.SlugDecoder); ok {
				fvs := []string{}
				for f, v := range slugger.PrimaryColumnValuesBySlug(id) {
					if f == "id" {
						fvs = append([]string{v}, fvs...)
					} else {
						if f != "version" {
							fvs = append(fvs, v)
						}
					}
				}
				promptID = strings.Join(fvs, "_")
			}

			onclick := web.Plaid().
				EventFunc(actions.DeleteConfirmation).
				Query(presets.ParamID, id).
				Query("all_versions", true).
				Query("prompt_id", promptID)
			if presets.IsInDialog(ctx) {
				onclick.URL(ctx.R.RequestURI).
					Query(presets.ParamOverlay, actions.Dialog).
					Query(presets.ParamInDialog, true).
					Query(presets.ParamListingQueries, ctx.Queries().Encode())
			}
			return vuetify.VListItem(
				web.Slot(
					vuetify.VIcon("mdi-delete"),
				).Name("prepend"),
				vuetify.VListItemTitle(htmlgo.Text(msgr.Delete)),
			).Attr("@click", onclick.Go())
		})
		// rewrite Deleter to ignore version condition
		ed.DeleteFunc(makeDeleteFunc(db))
	}

	setter := makeSetVersionSetterFunc(db)
	ed.WrapSetterFunc(setter)
	creating.WrapSetterFunc(setter)

	m.Listing().Field(ListingFieldDraftCount).ComponentFunc(draftCountFunc(db))
	m.Listing().Field(ListingFieldLive).ComponentFunc(liveFunc(db))

	configureVersionListDialog(db, pb, m)
}

func makeDeleteFunc(db *gorm.DB) presets.DeleteFunc {
	return func(obj interface{}, id string, ctx *web.EventContext) (err error) {
		allVersions := ctx.R.URL.Query().Get("all_versions") == "true"

		wh := db.Model(obj)

		if id != "" {
			if slugger, ok := obj.(presets.SlugDecoder); ok {
				cs := slugger.PrimaryColumnValuesBySlug(id)
				for key, value := range cs {
					if allVersions && key == "version" {
						continue
					}
					wh = wh.Where(fmt.Sprintf("%s = ?", key), value)
				}
			} else {
				wh = wh.Where("id =  ?", id)
			}
		}

		return wh.Delete(obj).Error
	}
}

func makeSearchFunc(db *gorm.DB) func(searcher presets.SearchFunc) presets.SearchFunc {
	return func(searcher presets.SearchFunc) presets.SearchFunc {
		return func(model interface{}, params *presets.SearchParams, ctx *web.EventContext) (r interface{}, totalCount int, err error) {
			stmt := &gorm.Statement{DB: db}
			stmt.Parse(model)
			tn := stmt.Schema.Table

			var pks []string
			condition := ""
			for _, f := range stmt.Schema.Fields {
				if f.Name == "DeletedAt" {
					condition = "WHERE deleted_at IS NULL"
				}
			}
			for _, f := range stmt.Schema.PrimaryFields {
				if f.Name != "Version" {
					pks = append(pks, f.DBName)
				}
			}
			pkc := strings.Join(pks, ",")

			sql := fmt.Sprintf(`
			(%s, version) IN (
				SELECT %s, version
				FROM (
					SELECT %s, version,
						ROW_NUMBER() OVER (PARTITION BY %s ORDER BY CASE WHEN status = '%s' THEN 0 ELSE 1 END, version DESC) as rn
					FROM %s %s
				) subquery
				WHERE subquery.rn = 1
			)`, pkc, pkc, pkc, pkc, StatusOnline, tn, condition)

			con := presets.SQLCondition{
				Query: sql,
			}
			params.SQLConditions = append(params.SQLConditions, &con)

			return searcher(model, params, ctx)
		}
	}
}

func makeSetVersionSetterFunc(db *gorm.DB) func(presets.SetterFunc) presets.SetterFunc {
	return func(in presets.SetterFunc) presets.SetterFunc {
		return func(obj interface{}, ctx *web.EventContext) {
			if ctx.Param(presets.ParamID) == "" {
				version := fmt.Sprintf("%s-v01", db.NowFunc().Format("2006-01-02"))
				if err := reflectutils.Set(obj, "Version.Version", version); err != nil {
					return
				}
				if err := reflectutils.Set(obj, "Version.VersionName", version); err != nil {
					return
				}
			}
			if in != nil {
				in(obj, ctx)
			}
		}
	}
}

func (b *Builder) Install(pb *presets.Builder) error {
	pb.FieldDefaults(presets.LIST).
		FieldType(Status{}).
		ComponentFunc(StatusListFunc())

	pb.I18n().
		RegisterForModule(language.English, I18nPublishKey, Messages_en_US).
		RegisterForModule(language.SimplifiedChinese, I18nPublishKey, Messages_zh_CN).
		RegisterForModule(language.Japanese, I18nPublishKey, Messages_ja_JP)

	utils.Install(pb)
	for _, f := range b.afterInstallFuncs {
		f()
	}
	return nil
}

func (b *Builder) ContextValueFuncs(vs ...ContextValueFunc) *Builder {
	b.ctxValueProviders = append(b.ctxValueProviders, vs...)
	return b
}

func (b *Builder) WithContextValues(ctx context.Context) context.Context {
	for _, v := range b.ctxValueProviders {
		ctx = v(ctx)
	}
	return ctx
}

// 幂等
func (b *Builder) Publish(record interface{}, ctx context.Context) (err error) {
	err = utils.Transact(b.db, func(tx *gorm.DB) (err error) {
		// publish content
		if r, ok := record.(PublishInterface); ok {
			var objs []*PublishAction
			objs, err = r.GetPublishActions(b.db, ctx, b.storage)
			if err != nil {
				return
			}
			if err = UploadOrDelete(objs, b.storage); err != nil {
				return
			}
		}

		// update status
		if r, ok := record.(StatusInterface); ok {
			now := b.db.NowFunc()
			if version, ok := record.(VersionInterface); ok {
				var modelSchema *schema.Schema
				modelSchema, err = schema.Parse(record, &sync.Map{}, b.db.NamingStrategy)
				if err != nil {
					return
				}
				scope := setPrimaryKeysConditionWithoutVersion(b.db.Model(reflect.New(modelSchema.ModelType).Interface()), record, modelSchema).Where("version <> ? AND status = ?", version.GetVersion(), StatusOnline)

				oldVersionUpdateMap := make(map[string]interface{})
				if _, ok := record.(ScheduleInterface); ok {
					oldVersionUpdateMap["scheduled_end_at"] = nil
					oldVersionUpdateMap["actual_end_at"] = &now
				}
				if _, ok := record.(ListInterface); ok {
					oldVersionUpdateMap["list_deleted"] = true
				}
				oldVersionUpdateMap["status"] = StatusOffline
				if err = scope.Updates(oldVersionUpdateMap).Error; err != nil {
					return
				}
			}
			updateMap := make(map[string]interface{})

			if r, ok := record.(ScheduleInterface); ok {
				r.SetPublishedAt(&now)
				r.SetScheduledStartAt(nil)
				updateMap["scheduled_start_at"] = r.GetScheduledStartAt()
				updateMap["actual_start_at"] = r.GetPublishedAt()
			}
			if _, ok := record.(ListInterface); ok {
				updateMap["list_updated"] = true
			}
			updateMap["status"] = StatusOnline
			updateMap["online_url"] = r.GetOnlineUrl()
			if err = b.db.Model(record).Updates(updateMap).Error; err != nil {
				return
			}
		}

		// publish callback
		if r, ok := record.(AfterPublishInterface); ok {
			if err = r.AfterPublish(b.db, b.storage, ctx); err != nil {
				return
			}
		}
		return
	})
	return
}

func (b *Builder) UnPublish(record interface{}, ctx context.Context) (err error) {
	err = utils.Transact(b.db, func(tx *gorm.DB) (err error) {
		// unpublish content
		if r, ok := record.(UnPublishInterface); ok {
			var objs []*PublishAction
			objs, err = r.GetUnPublishActions(b.db, ctx, b.storage)
			if err != nil {
				return
			}
			if err = UploadOrDelete(objs, b.storage); err != nil {
				return
			}
		}

		// update status
		if _, ok := record.(StatusInterface); ok {
			updateMap := make(map[string]interface{})
			if r, ok := record.(ScheduleInterface); ok {
				now := b.db.NowFunc()
				r.SetUnPublishedAt(&now)
				r.SetScheduledEndAt(nil)
				updateMap["scheduled_end_at"] = r.GetScheduledEndAt()
				updateMap["actual_end_at"] = r.GetUnPublishedAt()
			}
			if _, ok := record.(ListInterface); ok {
				updateMap["list_deleted"] = true
			}
			updateMap["status"] = StatusOffline
			if err = b.db.Model(record).Updates(updateMap).Error; err != nil {
				return
			}
		}

		// unpublish callback
		if r, ok := record.(AfterUnPublishInterface); ok {
			if err = r.AfterUnPublish(b.db, b.storage, ctx); err != nil {
				return
			}
		}
		return
	})
	return
}

func UploadOrDelete(objs []*PublishAction, storage oss.StorageInterface) (err error) {
	for _, obj := range objs {
		if obj.IsDelete {
			fmt.Printf("deleting %s \n", obj.Url)
			err = storage.Delete(obj.Url)
		} else {
			fmt.Printf("uploading %s \n", obj.Url)
			_, err = storage.Put(obj.Url, strings.NewReader(obj.Content))
		}
		if err != nil {
			return
		}
	}
	return nil
}

func setPrimaryKeysConditionWithoutVersion(db *gorm.DB, record interface{}, s *schema.Schema) *gorm.DB {
	querys := []string{}
	args := []interface{}{}
	for _, p := range s.PrimaryFields {
		if p.Name == "Version" {
			continue
		}
		val, _ := p.ValueOf(db.Statement.Context, reflect.ValueOf(record))
		querys = append(querys, fmt.Sprintf("%s = ?", strcase.ToSnake(p.Name)))
		args = append(args, val)
	}
	return db.Where(strings.Join(querys, " AND "), args...)
}
