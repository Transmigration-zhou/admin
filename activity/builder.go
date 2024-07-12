package activity

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/qor5/admin/v3/presets"
	"github.com/qor5/web/v3"
	"github.com/qor5/x/v3/perm"
	. "github.com/qor5/x/v3/ui/vuetify"
	. "github.com/theplant/htmlgo"
	"gorm.io/gorm"
)

const (
	Create = 1 << iota
	Delete
	Update
)

type User struct {
	ID     uint
	Name   string
	Avatar string
}

type CurrentUserFunc func(ctx context.Context) *User

// @snippet_begin(ActivityBuilder)
// Builder struct contains all necessary fields
type Builder struct {
	db              *gorm.DB                  // global db
	lmb             *presets.ModelBuilder     // log model builder
	models          []*ModelBuilder           // registered model builders
	tabHeading      func(*ActivityLog) string // tab heading format
	permPolicy      *perm.PolicyBuilder       // permission policy
	logModelInstall presets.ModelInstallFunc  // log model install
	currentUserFunc CurrentUserFunc
}

// @snippet_end

func (b *Builder) ModelInstall(pb *presets.Builder, m *presets.ModelBuilder) error {
	// Register the model
	b.RegisterModel(m)

	m.RegisterEventFunc(createNoteEvent, createNoteAction(b, m))
	m.RegisterEventFunc(updateUserNoteEvent, updateUserNoteAction(b, m))
	m.RegisterEventFunc(deleteNoteEvent, deleteNoteAction(b, m))
	// m.Listing().Field("Notes").ComponentFunc(noteFunc(db, m))

	return nil
}

func (ab *Builder) WrapLogModelInstall(w func(presets.ModelInstallFunc) presets.ModelInstallFunc) *Builder {
	ab.logModelInstall = w(ab.logModelInstall)
	return ab
}

func (ab *Builder) PermPolicy(v *perm.PolicyBuilder) *Builder {
	ab.permPolicy = v
	return ab
}

func (ab *Builder) CurrentUserFunc(v CurrentUserFunc) *Builder {
	ab.currentUserFunc = v
	return ab
}

// New initializes a new Builder instance with a provided database connection and an optional activity log model.
func New(db *gorm.DB) *Builder {
	ab := &Builder{
		db: db,
	}

	ab.logModelInstall = ab.defaultLogModelInstall

	ab.permPolicy = perm.PolicyFor(perm.Anybody).WhoAre(perm.Denied).
		ToDo(presets.PermUpdate, presets.PermDelete, presets.PermCreate).On("*:activity_logs").On("*:activity_logs:*")

	return ab
}

func (ab *Builder) GetActivityLogs(m interface{}, keys string, db *gorm.DB) []*ActivityLog {
	if keys == "" {
		keys = ab.MustGetModelBuilder(m).KeysValue(m)
	}
	var logs []*ActivityLog
	err := db.Where("model_name = ? AND model_keys = ?", modelName(m), keys).
		Order("created_at DESC").
		Find(&logs).Error
	if err != nil {
		return nil
	}
	return logs
}

// RegisterModels register mutiple models
func (ab *Builder) RegisterModels(models ...any) *Builder {
	for _, model := range models {
		ab.RegisterModel(model)
	}
	return ab
}

// RegisterModel Model register a model and return model builder
func (ab *Builder) RegisterModel(m any) (mb *ModelBuilder) {
	if m, exist := ab.GetModelBuilder(m); exist {
		return m
	}

	model := getBasicModel(m)
	if model == nil {
		panic(fmt.Sprintf("%v is nil", m))
	}

	reflectType := reflect.Indirect(reflect.ValueOf(model)).Type()
	if reflectType.Kind() != reflect.Struct {
		panic(fmt.Sprintf("%v is not a struct", reflectType.Name()))
	}

	keys := getPrimaryKey(reflectType)
	mb = &ModelBuilder{
		typ:      reflectType,
		activity: ab,

		keys:          keys,
		ignoredFields: keys,
	}
	ab.models = append(ab.models, mb)

	if presetModel, ok := m.(*presets.ModelBuilder); ok {
		ab.installModelBuilder(mb, presetModel)
	}

	return mb
}

func humanContent(log *ActivityLog) string {
	return fmt.Sprintf("%s: %s", log.Action, log.Comment)
}

func (ab *Builder) installModelBuilder(mb *ModelBuilder, presetModel *presets.ModelBuilder) {
	mb.presetModel = presetModel

	editing := presetModel.Editing()
	d := presetModel.Detailing()

	lb := presetModel.Listing()

	db := ab.db

	d.Field(DetailFieldTimeline).ComponentFunc(func(obj any, field *presets.FieldContext, ctx *web.EventContext) HTMLComponent {
		return web.Portal(ab.timelineList(obj, "", db)).Name(TimelinePortalName)
	})

	lb.Field(ListFieldNotes).ComponentFunc(func(obj interface{}, field *presets.FieldContext, ctx *web.EventContext) HTMLComponent {
		rt := modelName(presetModel.NewModel())
		ri := mb.KeysValue(obj)
		user := ab.currentUserFunc(ctx.R.Context())
		count, _ := GetUnreadNotesCount(db, user.ID, rt, ri)

		return Td(
			If(count > 0,
				VBadge().Content(count).Color("red"),
			).Else(
				Text(""),
			),
		)
	}).Label("Unread Notes")

	editing.WrapSaveFunc(func(in presets.SaveFunc) presets.SaveFunc {
		return func(obj any, id string, ctx *web.EventContext) (err error) {
			if mb.skip&Update != 0 && mb.skip&Create != 0 {
				return in(obj, id, ctx)
			}

			old, ok := findOld(obj, db)
			if err = in(obj, id, ctx); err != nil {
				return err
			}

			if (!ok || id == "") && mb.skip&Create == 0 {
				return mb.AddRecords(ActionCreate, ctx.R.Context(), obj)
			}

			if ok && id != "" && mb.skip&Update == 0 {
				return mb.AddEditRecordWithOld(ab.currentUserFunc(ctx.R.Context()), old, obj, db)
			}

			return
		}
	})

	editing.WrapDeleteFunc(func(in presets.DeleteFunc) presets.DeleteFunc {
		return func(obj any, id string, ctx *web.EventContext) (err error) {
			if mb.skip&Delete != 0 {
				return in(obj, id, ctx)
			}

			old, ok := findOldWithSlug(obj, id, db)
			if err = in(obj, id, ctx); err != nil {
				return err
			}

			if ok {
				return mb.AddRecords(ActionDelete, ctx.R.Context(), old)
			}

			return
		}
	})
}

func (ab *Builder) timelineList(obj any, keys string, db *gorm.DB) HTMLComponent {
	// Fetch combined timeline data
	logs := ab.GetActivityLogs(obj, keys, db)

	// Create VTimelineItems
	var timelineItems []HTMLComponent
	for _, item := range logs {
		timelineItems = append(timelineItems,
			Div(
				Div(
					Text(item.CreatedAt.Format("2 hours ago")),
				),
				Div(
					Div(
						VAvatar().Text(strings.ToUpper(item.Creator.Name)).Color("secondary").Class(
							"text-h6 rounded-lg").Size("x-small"),
						Div(
							Strong(item.Creator.Name).Class("ml-1").Style(
								"width: 100%; height: 20px; font-family: SF Pro; font-style: normal; font-weight: 510; font-size: 14px; line-height: 20px; display: flex; align-items: center; color: #9e9e9e;"),
							Div(Text(humanContent(item))).Class("text-caption").Style(
								"width: 100%; font-family: SF Pro; font-style: normal; font-weight: 400; font-size: 14px; line-height: 20px; color: #9e9e9e;"),
						).Class("detailsStyle").Style("display: flex; flex-direction: column; align-items: flex-start; padding: 0; width: 100%;"),
					).Class("contentStyle").Style("display: flex; flex-direction: row; align-items: flex-start; padding: 0; gap: 8px; width: 100%;"),
				),
			).Class("itemStyle").Style("width: 100%; color: #9e9e9e;"),
		)
	}

	return VTimeline(
		timelineItems...,
	)
}

// GetModelBuilder 	get model builder
func (ab *Builder) GetModelBuilder(v any) (*ModelBuilder, bool) {
	var isPreset bool
	if _, ok := v.(*presets.ModelBuilder); ok {
		isPreset = true
	}

	typ := reflect.Indirect(reflect.ValueOf(getBasicModel(v))).Type()
	for _, m := range ab.models {
		if m.typ == typ {
			if !isPreset {
				return m, true
			}

			if isPreset && m.presetModel == v {
				return m, true
			}
		}
	}
	return &ModelBuilder{}, false
}

// GetModelBuilder 	get model builder
func (ab *Builder) MustGetModelBuilder(v any) *ModelBuilder {
	mb, ok := ab.GetModelBuilder(v)
	if !ok {
		panic(fmt.Sprintf("model %v is not registered", v))
	}
	return mb
}

// GetModelBuilders 	get all model builders
func (ab *Builder) GetModelBuilders() []*ModelBuilder {
	return ab.models
}

// AddRecords add records log
func (ab *Builder) AddRecords(action string, ctx context.Context, vs ...any) error {
	if len(vs) == 0 {
		return errors.New("data are empty")
	}

	for _, v := range vs {
		if mb, ok := ab.GetModelBuilder(v); ok {
			if err := mb.AddRecords(action, ctx, v); err != nil {
				return err
			}
		}
	}

	return nil
}

// AddCustomizedRecord add customized record
func (ab *Builder) AddCustomizedRecord(action string, diff bool, ctx context.Context, obj any) error {
	if mb, ok := ab.GetModelBuilder(obj); ok {
		return mb.AddCustomizedRecord(action, diff, ctx, obj)
	}

	return fmt.Errorf("can't find model builder for %v", obj)
}

// AddViewRecord add view record
func (ab *Builder) AddViewRecord(creator *User, v any, db *gorm.DB) error {
	if mb, ok := ab.GetModelBuilder(v); ok {
		return mb.AddViewRecord(creator, v, db)
	}

	return fmt.Errorf("can't find model builder for %v", v)
}

// AddDeleteRecord	add delete record
func (ab *Builder) AddDeleteRecord(creator *User, v any, db *gorm.DB) error {
	if mb, ok := ab.GetModelBuilder(v); ok {
		return mb.AddDeleteRecord(creator, v, db)
	}

	return fmt.Errorf("can't find model builder for %v", v)
}

// AddSaverRecord will save a create log or a edit log
func (ab *Builder) AddSaveRecord(creator *User, now any, db *gorm.DB) error {
	if mb, ok := ab.GetModelBuilder(now); ok {
		return mb.AddSaveRecord(creator, now, db)
	}

	return fmt.Errorf("can't find model builder for %v", now)
}

// AddCreateRecord add create record
func (ab *Builder) AddCreateRecord(creator *User, v any, db *gorm.DB) error {
	if mb, ok := ab.GetModelBuilder(v); ok {
		return mb.AddCreateRecord(creator, v, db)
	}

	return fmt.Errorf("can't find model builder for %v", v)
}

// AddEditRecord add edit record
func (ab *Builder) AddEditRecord(creator *User, now any, db *gorm.DB) error {
	if mb, ok := ab.GetModelBuilder(now); ok {
		return mb.AddEditRecord(creator, now, db)
	}

	return fmt.Errorf("can't find model builder for %v", now)
}

// AddEditRecord add edit record
func (ab *Builder) AddEditRecordWithOld(creator *User, old, now any, db *gorm.DB) error {
	if mb, ok := ab.GetModelBuilder(now); ok {
		return mb.AddEditRecordWithOld(creator, old, now, db)
	}

	return fmt.Errorf("can't find model builder for %v", now)
}

// AddEditRecordWithOldAndContext add edit record
func (ab *Builder) AddEditRecordWithOldAndContext(ctx context.Context, old, now any) error {
	if mb, ok := ab.GetModelBuilder(now); ok {
		return mb.AddEditRecordWithOld(ab.currentUserFunc(ctx), old, now, ab.db)
	}

	return fmt.Errorf("can't find model builder for %v", now)
}

func (b *Builder) AutoMigrate() (r *Builder) {
	if err := AutoMigrate(b.db); err != nil {
		panic(err)
	}
	return b
}

func AutoMigrate(db *gorm.DB) (err error) {
	return db.AutoMigrate(&ActivityLog{})
}

func modelName(v any) string {
	segs := strings.Split(reflect.TypeOf(v).String(), ".")
	return segs[len(segs)-1]
}