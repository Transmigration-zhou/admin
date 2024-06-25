package presets

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/qor5/admin/v3/presets/actions"
	"github.com/qor5/web/v3"
	"github.com/qor5/web/v3/stateful"
	"github.com/qor5/x/v3/i18n"
	. "github.com/qor5/x/v3/ui/vuetify"
	vx "github.com/qor5/x/v3/ui/vuetifyx"
	"github.com/samber/lo"
	h "github.com/theplant/htmlgo"
)

func init() {
	stateful.RegisterActionableType((*ListingCompo)(nil))
}

type DisplayColumn struct {
	Name    string `json:"name"`
	Visible bool   `json:"visible"`
}

type ListingCompo struct {
	lb *ListingBuilderX `inject:""`

	CompoID            string          `json:"compo_id"`
	LongStyleSearchBox bool            `json:"long_style_search_box"`
	SelectedIDs        []string        `json:"selected_ids" query:"selected_ids"`
	Keyword            string          `json:"keyword" query:"keyword"`
	OrderBys           []ColOrderBy    `json:"order_bys" query:"order_bys"`
	Page               int64           `json:"page" query:"page"`
	PerPage            int64           `json:"per_page" query:"per_page"`
	DisplayColumns     []DisplayColumn `json:"display_columns" query:"display_columns"`
}

func (c *ListingCompo) CompoName() string {
	return fmt.Sprintf("ListingCompo:%s", c.CompoID)
}

const (
	listingLocals            = "listingLocals"
	listingLocalsSelectedIDs = "listingLocals.selected_ids"
)

func (c *ListingCompo) MarshalHTML(ctx context.Context) (r []byte, err error) {
	// TODO:
	// msgr := c.MustGetMessages(ctx)

	// ctx.WithContextValue(ctxInDialog, inDialog)

	// actionsComponent := b.actionsComponent(ctx, msgr, inDialog)
	// ctx.WithContextValue(ctxActionsComponent, actionsComponent)

	dataTable, err := c.dataTable(ctx)
	if err != nil {
		return nil, err
	}

	return stateful.Reloadable(c,
		web.Scope().
			VSlot(fmt.Sprintf("{ locals: %s }", listingLocals)).
			Init(fmt.Sprintf(`{
				currEditingListItemID: "",
				selected_ids: %s || [],
			}`, h.JSONString(c.SelectedIDs))).
			Observe(c.lb.mb.NotifModelsUpdated(), c.ReloadActionGo(ctx, nil)).
			Observe(c.lb.mb.NotifModelsDeleted(), fmt.Sprintf(`
				if (payload && payload.ids && payload.ids.length > 0) {
					%s = %s.filter(id => !payload.ids.includes(id));
				}
				%s`,
				listingLocalsSelectedIDs, listingLocalsSelectedIDs,
				c.ReloadActionGo(ctx, nil),
			)).
			Children(
				VCard().Elevation(0).Children(
					// b.filterTabs(ctx, inDialog),
					c.toolbarSearch(ctx),
					VCardText().Class("pa-2").Children(
						dataTable,
					),
				// b.footerCardActions(ctx),
				),
			),
	).MarshalHTML(ctx)
}

func (c *ListingCompo) textFieldSearch(ctx context.Context) h.HTMLComponent {
	if c.lb.keywordSearchOff {
		return nil
	}
	msgr := c.MustGetMessages(ctx)
	onChanged := func(quoteKeyword string) string {
		return strings.Replace(
			c.ReloadActionGo(ctx, func(cloned *ListingCompo) {
				cloned.Keyword = ""
			}),
			`"keyword": ""`,
			`"keyword": `+quoteKeyword,
			1,
		)
	}
	// TODO: isFocus 原来的目的是什么来着？
	return web.Scope().VSlot("{ locals }").Init(`{isFocus: false}`).Children(
		VTextField().
			Density(DensityCompact).
			Variant(FieldVariantOutlined).
			Label(msgr.Search).
			Flat(true).
			Clearable(true).
			HideDetails(true).
			SingleLine(true).
			ModelValue(c.Keyword).
			Attr("@keyup.enter", onChanged("$event.target.value")).
			Attr("@click:clear", onChanged(`""`)).
			Children(
				web.Slot(VIcon("mdi-magnify")).Name("append-inner"),
			),
	)
}

func (c *ListingCompo) filterSearch(ctx context.Context, fd vx.FilterData) h.HTMLComponent {
	if fd == nil {
		return nil
	}
	if !lo.ContainsBy(fd, func(d *vx.FilterItem) bool {
		return !d.Invisible
	}) {
		return nil
	}

	msgr := c.MustGetMessages(ctx)

	for _, d := range fd {
		d.Translations = vx.FilterIndependentTranslations{
			FilterBy: msgr.FilterBy(d.Label),
		}
	}

	ft := vx.FilterTranslations{}
	ft.Clear = msgr.FiltersClear
	ft.Add = msgr.FiltersAdd
	ft.Apply = msgr.FilterApply
	ft.Date.To = msgr.FiltersDateTo
	ft.Number.And = msgr.FiltersNumberAnd
	ft.Number.Equals = msgr.FiltersNumberEquals
	ft.Number.Between = msgr.FiltersNumberBetween
	ft.Number.GreaterThan = msgr.FiltersNumberGreaterThan
	ft.Number.LessThan = msgr.FiltersNumberLessThan
	ft.String.Equals = msgr.FiltersStringEquals
	ft.String.Contains = msgr.FiltersStringContains
	ft.MultipleSelect.In = msgr.FiltersMultipleSelectIn
	ft.MultipleSelect.NotIn = msgr.FiltersMultipleSelectNotIn

	return vx.VXFilter(fd).Translations(ft)
	// TODO:
	// if inDialog {
	// 	filter.UpdateModelValue(Zone[*ListingZone](ctx).Plaid().
	// 		URL(ctx.R.RequestURI).
	// 		StringQuery(web.Var("$event.encodedFilterData")).
	// 		Query("page", 1).
	// 		ClearMergeQuery(web.Var("$event.filterKeys")).
	// 		EventFunc(actions.ReloadList).
	// 		Go())
	// }
	// return filter
}

func (c *ListingCompo) toolbarSearch(ctx context.Context) h.HTMLComponent {
	evCtx := web.MustGetEventContext(ctx)
	// msgr := c.MustGetMessages(ctx)

	var filterSearch h.HTMLComponent
	if c.lb.filterDataFunc != nil {
		fd := c.lb.filterDataFunc(evCtx)
		fd.SetByQueryString(evCtx.R.URL.RawQuery) // TODO:
		filterSearch = c.filterSearch(ctx, fd)
	}

	tfSearch := VResponsive().Children(
		c.textFieldSearch(ctx),
	)
	if filterSearch != nil || !c.LongStyleSearchBox {
		tfSearch.MaxWidth(200).MinWidth(200).Class("mr-4")
	} else {
		tfSearch.Width(100)
	}
	return VToolbar().Flat(true).Color("surface").AutoHeight(true).Class("pa-2").Children(
		tfSearch,
		filterSearch,
	)
}

func (c *ListingCompo) defaultCellWrapperFunc(cell h.MutableAttrHTMLComponent, id string, obj any, dataTableID string) h.HTMLComponent {
	if c.lb.mb.hasDetailing && !c.lb.mb.detailing.drawer {
		cell.SetAttr("@click.self", web.Plaid().PushStateURL(c.lb.mb.Info().DetailingHref(id)).Go())
		return cell
	}

	event := actions.Edit
	if c.lb.mb.hasDetailing {
		event = actions.DetailingDrawer
	}
	onClick := web.Plaid().EventFunc(event).Query(ParamID, id)
	// TODO: how to auto open in dialog ?
	// if inDialog {
	// 	onclick.URL(ctx.R.RequestURI).
	// 		Query(ParamOverlay, actions.Dialog).
	// 		Query(ParamInDialog, true).
	// 		Query(ParamListingQueries, ctx.Queries().Encode())
	// }
	// TODO: 需要更优雅的方式
	cell.SetAttr("@click.self", fmt.Sprintf(`%s; %s.currEditingListItemID="%s-%s"`, onClick.Go(), listingLocals, dataTableID, id))
	return cell
}

func (c *ListingCompo) dataTable(ctx context.Context) (h.HTMLComponent, error) {
	if c.lb.Searcher == nil {
		return nil, errors.New("function Searcher is not set")
	}

	evCtx := web.MustGetEventContext(ctx)
	msgr := c.MustGetMessages(ctx)

	searchParams := &SearchParams{
		PageURL:       evCtx.R.URL,
		SQLConditions: c.lb.conditions,
	}

	if !c.lb.keywordSearchOff {
		searchParams.KeywordColumns = c.lb.searchColumns
		searchParams.Keyword = c.Keyword
	}

	orderBys := lo.Map(c.OrderBys, func(ob ColOrderBy, _ int) ColOrderBy {
		ob.OrderBy = strings.ToUpper(ob.OrderBy)
		if ob.OrderBy != OrderByASC && ob.OrderBy != OrderByDESC {
			ob.OrderBy = OrderByDESC
		}
		return ob
	})
	orderableFieldMap := make(map[string]string)
	for _, v := range c.lb.orderableFields {
		orderableFieldMap[v.FieldName] = v.DBColumn
	}
	dbOrderBys := []string{}
	for _, ob := range orderBys {
		dbCol, ok := orderableFieldMap[ob.FieldName]
		if !ok {
			continue
		}
		dbBy := ob.OrderBy
		dbOrderBys = append(dbOrderBys, fmt.Sprintf("%s %s", dbCol, dbBy))
	}
	var orderBySQL string
	if len(dbOrderBys) == 0 {
		if c.lb.orderBy != "" {
			orderBySQL = c.lb.orderBy
		} else {
			orderBySQL = fmt.Sprintf("%s %s", c.lb.mb.primaryField, OrderByDESC)
		}
	} else {
		orderBySQL = strings.Join(dbOrderBys, ", ")
	}
	searchParams.OrderBy = orderBySQL

	if !c.lb.disablePagination {
		perPage := c.PerPage // TODO: sync cookie ?
		if perPage > 1000 {
			perPage = 1000
		}
		searchParams.PerPage = cmp.Or(perPage, 10) // TODO:
		searchParams.Page = cmp.Or(c.Page, 1)
	}

	var fd vx.FilterData
	if c.lb.filterDataFunc != nil {
		// TODO: how to stateful?
		fd = c.lb.filterDataFunc(evCtx)
		cond, args := fd.SetByQueryString(evCtx.R.URL.RawQuery)
		searchParams.SQLConditions = append(searchParams.SQLConditions, &SQLCondition{
			Query: cond,
			Args:  args,
		})
	}

	objs, totalCount, err := c.lb.Searcher(c.lb.mb.NewModelSlice(), searchParams, evCtx)
	if err != nil {
		return nil, err
	}

	btnConfigColumns, columns := c.displayColumns(ctx)

	dataTable := vx.DataTableX(objs).
		HeadCellWrapperFunc(func(cell h.MutableAttrHTMLComponent, field string, title string) h.HTMLComponent {
			if _, exists := orderableFieldMap[field]; !exists {
				return cell
			}

			orderBy, orderByIdx, exists := lo.FindIndexOf(orderBys, func(ob ColOrderBy) bool {
				return ob.FieldName == field
			})
			if !exists {
				orderBy = ColOrderBy{
					FieldName: field,
					OrderBy:   OrderByDESC,
				}
			}

			icon := "mdi-arrow-down"
			if orderBy.OrderBy == OrderByASC {
				icon = "mdi-arrow-up"
			}
			return h.Th("").Style("cursor: pointer; white-space: nowrap;").
				Attr("@click", c.ReloadActionGo(ctx, func(cloned *ListingCompo) {
					if orderBy.OrderBy == OrderByASC {
						orderBy.OrderBy = OrderByDESC
					} else {
						orderBy.OrderBy = OrderByASC
					}
					if exists {
						if orderBy.OrderBy == OrderByASC {
							cloned.OrderBys = append(cloned.OrderBys[:orderByIdx], cloned.OrderBys[orderByIdx+1:]...)
						} else {
							cloned.OrderBys[orderByIdx] = orderBy
						}
					} else {
						cloned.OrderBys = append(cloned.OrderBys, orderBy)
					}
				})).
				Children(
					h.Span(title).Style("text-decoration: underline;"),
					h.Span("").StyleIf("visibility: hidden;", !exists).Children(
						VIcon(icon).Size(SizeSmall),
						h.Span(fmt.Sprint(orderByIdx+1)),
					),
				)
		}).
		RowWrapperFunc(func(row h.MutableAttrHTMLComponent, id string, obj any, dataTableID string) h.HTMLComponent {
			// TODO: how to cancel active ? 不可能都根据 vars.presetsRightDrawer 去 cancel
			row.SetAttr(":class", fmt.Sprintf(`{
					"vx-list-item--active primary--text": vars.presetsRightDrawer && %s.currEditingListItemID==="%s-%s",
				}`, listingLocals, dataTableID, id,
			))
			return row
		}).
		RowMenuHead(btnConfigColumns).
		// RowMenuItemFuncs(c.lb.RowMenu().listingItemFuncs(evCtx)...). // TODO:
		CellWrapperFunc(
			lo.If(c.lb.cellWrapperFunc != nil, c.lb.cellWrapperFunc).Else(c.defaultCellWrapperFunc),
		).
		VarSelectedIDs(
			lo.If(len(c.lb.bulkActions) > 0, listingLocalsSelectedIDs).Else(""),
		).
		SelectedCountLabel(msgr.ListingSelectedCountNotice).
		ClearSelectionLabel(msgr.ListingClearSelection)

	for _, col := range columns {
		if !col.Visible {
			continue
		}
		// fill in empty compFunc and setter func with default
		f := c.lb.getFieldOrDefault(col.Name)
		dataTable.Column(col.Name).Title(col.Label).CellComponentFunc(c.lb.cellComponentFunc(f))
	}

	if c.lb.disablePagination {
		return dataTable, nil
	}

	var dataTableAdditions h.HTMLComponent
	if totalCount <= 0 {
		dataTableAdditions = h.Div().Class("mt-10 text-center grey--text text--darken-2").Children(
			h.Text(msgr.ListingNoRecordToShow),
		)
	} else {
		dataTableAdditions = h.Div().Class("mt-2").Children(
			vx.VXTablePagination().
				Total(int64(totalCount)).
				CurrPage(searchParams.Page).
				PerPage(searchParams.PerPage).
				CustomPerPages([]int64{c.lb.perPage}).
				PerPageText(msgr.PaginationRowsPerPage).
				OnSelectPerPage(strings.Replace(
					c.ReloadActionGo(ctx, func(cloned *ListingCompo) {
						cloned.PerPage = -1
					}),
					`"per_page": -1`,
					`"per_page": parseInt($event, 10)`,
					1,
				)).
				OnPrevPage(c.ReloadActionGo(ctx, func(cloned *ListingCompo) {
					cloned.Page = searchParams.Page - 1
				})).
				OnNextPage(c.ReloadActionGo(ctx, func(cloned *ListingCompo) {
					cloned.Page = searchParams.Page + 1
				})),
		)
	}
	return h.Components(dataTable, dataTableAdditions), nil
}

type DisplayColumnWrapper struct {
	DisplayColumn
	Label string `json:"label"`
}

func (c *ListingCompo) displayColumns(ctx context.Context) (btnConfigure h.HTMLComponent, wrappers []DisplayColumnWrapper) {
	evCtx := web.MustGetEventContext(ctx)
	msgr := c.MustGetMessages(ctx)

	var availableColumns []DisplayColumn
	for _, f := range c.lb.fields {
		if c.lb.mb.Info().Verifier().Do(PermList).SnakeOn("f_"+f.name).WithReq(evCtx.R).IsAllowed() != nil {
			continue
		}
		availableColumns = append(availableColumns, DisplayColumn{
			Name:    f.name,
			Visible: true,
		})
	}

	// if there is abnormal data, restore the default
	if len(c.DisplayColumns) != len(availableColumns) ||
		// names not match
		!lo.EveryBy(c.DisplayColumns, func(dc DisplayColumn) bool {
			return lo.ContainsBy(availableColumns, func(ac DisplayColumn) bool {
				return ac.Name == dc.Name
			})
		}) {
		// TODO: 对于状态的修正是否应该在 MarshalHTML 的头部提前统一处理？
		c.DisplayColumns = availableColumns
	}

	allInvisible := lo.EveryBy(c.DisplayColumns, func(dc DisplayColumn) bool {
		return !dc.Visible
	})
	for _, col := range c.DisplayColumns {
		if allInvisible {
			col.Visible = true
		}
		wrappers = append(wrappers, DisplayColumnWrapper{
			DisplayColumn: col,
			Label:         i18n.PT(evCtx.R, ModelsI18nModuleKey, c.lb.mb.label, c.lb.mb.getLabel(c.lb.Field(col.Name).NameLabel)),
		})
	}

	if !c.lb.selectableColumns {
		return nil, wrappers
	}

	return web.Scope().
			VSlot("{ locals }").
			Init(fmt.Sprintf(`{selectColumnsMenu: false, displayColumns: %s}`, h.JSONString(wrappers))).
			Children(
				VMenu().CloseOnContentClick(false).Width(240).Attr("v-model", "locals.selectColumnsMenu").Children(
					web.Slot().Name("activator").Scope("{ props }").Children(
						VBtn("").Icon("mdi-cog").Attr("v-bind", "props").Variant(VariantText).Size(SizeSmall),
					),
					VList().Density(DensityCompact).Children(
						h.Tag("vx-draggable").Attr("item-key", "name").Attr("v-model", "locals.displayColumns", "handle", ".handle", "animation", "300").Children(
							h.Template().Attr("#item", " { element } ").Children(
								VListItem(
									VListItemTitle(
										VSwitch().Density(DensityCompact).Color("primary").Class(" mt-2 ").Attr(
											"v-model", "element.visible",
											":label", "element.label",
										),
										VIcon("mdi-reorder-vertical").Class("handle cursor-grab mt-4"),
									).Class("d-flex justify-space-between "),
									VDivider(),
								),
							),
						),
						VListItem().Class("d-flex justify-space-between").Children(
							VBtn(msgr.Cancel).Elevation(0).Attr("@click", `locals.selectColumnsMenu = false`),
							VBtn(msgr.OK).Elevation(0).Color("primary").Attr("@click", fmt.Sprintf(`
								locals.selectColumnsMenu = false; 
								%s`,
								strings.Replace(
									c.ReloadActionGo(ctx, func(cloned *ListingCompo) {
										cloned.DisplayColumns = []DisplayColumn{}
									}),
									`"display_columns": []`,
									`"display_columns": locals.displayColumns.map(({ label, ...rest }) => rest)`,
									1,
								),
							)),
						),
					),
				),
			),
		wrappers
}

func (c *ListingCompo) ReloadActionGo(ctx context.Context, f func(cloned *ListingCompo)) string {
	return strings.Replace( // TODO: 这种 replace 的方式会影响到 sync_query 机制，那块需要改进
		stateful.ReloadAction(ctx, c, func(cloned *ListingCompo) {
			cloned.SelectedIDs = []string{}
			if f != nil {
				f(cloned)
			}
		}).Go(),
		`"selected_ids": []`,
		`"selected_ids": `+listingLocalsSelectedIDs,
		1,
	)
}

func (c *ListingCompo) MustGetMessages(ctx context.Context) *Messages {
	return MustGetMessages(web.MustGetEventContext(ctx).R)
}
