package admin_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/qor5/admin/v3/cmd/qor5/website-template/admin"
	"github.com/qor5/web/v3/multipartestutils"
	"github.com/theplant/gofixtures"
	"github.com/theplant/testenv"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	TestDB *gorm.DB
	SqlDB  *sql.DB
)

func TestMain(m *testing.M) {
	env, err := testenv.New().DBEnable(true).SetUp()
	if err != nil {
		panic(err)
	}
	defer env.TearDown()
	TestDB = env.DB
	TestDB.Logger = TestDB.Logger.LogMode(logger.Info)
	SqlDB, _ = TestDB.DB()
	m.Run()
}

var data = gofixtures.Data(gofixtures.Sql(`
INSERT INTO public.page_builder_pages (id, created_at, updated_at, deleted_at, title, slug, category_id, seo, status, online_url, scheduled_start_at, scheduled_end_at, actual_start_at, actual_end_at, version, version_name, parent_version, locale_code) VALUES (2, '2024-06-08 02:14:07.024850 +00:00', '2024-06-08 02:14:07.024850 +00:00', null, 'My first page', 'my-first-page', 0, '{"OpenGraphImageFromMediaLibrary":{"ID":0,"Url":"","VideoLink":"","FileName":"","Description":""}}', 'draft', '', null, null, null, null, '2024-06-08-v01', '2024-06-08-v01', '', '');
INSERT INTO public.page_builder_containers (id, created_at, updated_at, deleted_at, page_id, page_version, page_model_name, model_name, model_id, display_order, shared, hidden, display_name, locale_code, localize_from_model_id) VALUES (1, '2024-06-08 03:04:15.439286 +00:00', '2024-06-08 03:04:15.439286 +00:00', null, 2, '2024-06-08-v01', 'pages', 'MyHeader', 1, 1, false, false, 'MyHeader', '', 0);

`, []string{"page_builder_pages", "page_builder_containers"}))

func TestAll(t *testing.T) {

	mux := admin.Router(TestDB)

	cases := []multipartestutils.TestCase{
		{
			Name:  "index page",
			Debug: true,
			ReqFunc: func() *http.Request {
				return httptest.NewRequest("GET", "/admin/pages", nil)
			},
			ExpectPageBodyContainsInOrder: []string{"My first page"},
		},
		{
			Name:  "add container to page",
			Debug: true,
			ReqFunc: func() *http.Request {
				data.TruncatePut(SqlDB)
				req := multipartestutils.NewMultipartBuilder().
					PageURL("/admin/page_builder/pages/editors/2_2024-06-08-v01?__execute_event__=page_builder_AddContainerEvent&containerName=MyHeader&modelName=MyHeader&tab=Elements").
					BuildEventFuncRequest()
				return req
			},
			ExpectRunScriptContainsInOrder: []string{"page_builder_ReloadRenderPageOrTemplateEvent"},
		},
		{
			Name:  "add container to page",
			Debug: true,
			ReqFunc: func() *http.Request {
				data.TruncatePut(SqlDB)
				req := multipartestutils.NewMultipartBuilder().
					PageURL("/admin/page_builder/pages/editors/2_2024-06-08-v01?__execute_event__=page_builder_AddContainerEvent&containerName=MyHeader&modelName=MyHeader&tab=Elements").
					BuildEventFuncRequest()
				return req
			},
			ExpectRunScriptContainsInOrder: []string{"page_builder_ReloadRenderPageOrTemplateEvent"},
		},
		{
			Name:  "add menu items to header",
			Debug: true,
			ReqFunc: func() *http.Request {
				data.TruncatePut(SqlDB)
				req := multipartestutils.NewMultipartBuilder().
					PageURL("/admin/page_builder/pages/editors/2_2024-06-08-v01?__execute_event__=page_builder_ReloadRenderPageOrTemplateEvent&containerDataID=my-headers_1&containerID=1&tab=Layers").
					AddField("MenuItems[0].Text", "123").
					AddField("MenuItems[0].Link", "123").
					AddField("MenuItems[1].Text", "456").
					AddField("MenuItems[1].Link", "456").
					BuildEventFuncRequest()
				return req
			},
			ExpectRunScriptContainsInOrder: []string{"page_builder_ReloadRenderPageOrTemplateEvent"},
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			multipartestutils.RunCase(t, c, mux)
		})
	}
}
