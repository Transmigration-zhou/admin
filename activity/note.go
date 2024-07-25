package activity

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/samber/lo"
	"gorm.io/gorm"
)

type NoteCount struct {
	ModelName        string
	ModelKeys        string
	UnreadNotesCount int64
	TotalNotesCount  int64
}

func getNotesCounts(db *gorm.DB, tablePrefix string, creatorID string, modelName string, modelKeyses []string) ([]*NoteCount, error) {
	if creatorID == "" {
		return nil, errors.New("creatorID is required")
	}

	tableName := tablePrefix + ParseTableNameWithDB(db, &ActivityLog{})

	args := []any{
		ActionNote,
	}

	var explictWhere string
	if modelName != "" {
		explictWhere = ` AND model_name = ?`
		args = append(args, modelName)
	}
	if len(modelKeyses) > 0 {
		explictWhere += ` AND model_keys IN (?)`
		args = append(args, modelKeyses)
	}

	args = append(args, ActionLastView, creatorID)

	if modelName != "" {
		args = append(args, modelName)
	}
	if len(modelKeyses) > 0 {
		args = append(args, modelKeyses)
	}

	args = append(args, creatorID)

	raw := fmt.Sprintf(`
	WITH NoteRecords AS (
		SELECT model_name, model_keys, created_at, creator_id
		FROM %s
		WHERE action = ? AND deleted_at IS NULL
			%s
	),
	LastViewedAts AS (
		SELECT model_name, model_keys, MAX(updated_at) AS last_viewed_at
		FROM %s
		WHERE action = ? AND creator_id = ? AND deleted_at IS NULL
			%s
		GROUP BY model_name, model_keys
	)
	SELECT
		n.model_name,
		n.model_keys,
		COUNT(CASE WHEN n.creator_id <> ? AND (lva.last_viewed_at IS NULL OR n.created_at > lva.last_viewed_at) THEN 1 END) AS unread_notes_count,
		COUNT(*) AS total_notes_count
	FROM NoteRecords n
	LEFT JOIN LastViewedAts lva
		ON n.model_name = lva.model_name
		AND n.model_keys = lva.model_keys
	GROUP BY n.model_name, n.model_keys;`, tableName, explictWhere, tableName, explictWhere)

	counts := []*NoteCount{}
	if err := db.Raw(raw, args...).Scan(&counts).Error; err != nil {
		return nil, err
	}
	return counts, nil
}

func markAllNotesAsRead(db *gorm.DB, tablePrefix string, creatorID string) error {
	tableName := tablePrefix + ParseTableNameWithDB(db, &ActivityLog{})

	return db.Transaction(func(tx *gorm.DB) error {
		var results []struct {
			ModelName    string
			ModelKeys    string
			MaxCreatedAt time.Time
		}
		if err := db.Raw(fmt.Sprintf(`
			SELECT model_name, model_keys, MAX(created_at) AS max_created_at
			FROM %s
			WHERE action = ? AND deleted_at IS NULL
			GROUP BY model_name, model_keys;
			`, tableName), ActionNote,
		).Scan(&results).Error; err != nil {
			return errors.Wrap(err, "")
		}

		if len(results) <= 0 {
			return nil
		}

		if err := tx.Unscoped().Where("creator_id = ? AND action = ?", creatorID, ActionLastView).Delete(&ActivityLog{}).Error; err != nil {
			return errors.Wrap(err, "")
		}

		var logs []ActivityLog
		for _, v := range results {
			log := ActivityLog{
				CreatorID: creatorID,
				Action:    ActionLastView,
				Hidden:    true,
				ModelName: v.ModelName,
				ModelKeys: v.ModelKeys,
			}
			log.CreatedAt = v.MaxCreatedAt
			log.UpdatedAt = v.MaxCreatedAt
			logs = append(logs, log)
		}

		if err := tx.Create(&logs).Error; err != nil {
			return errors.Wrap(err, "")
		}

		return nil
	})
}

func sqlConditionHasUnreadNotes(db *gorm.DB, tablePrefix string, creatorID string, modelName string, columns []string, sep string, columnPrefix string) string {
	a := strings.Join(lo.Map(columns, func(v string, _ int) string {
		return fmt.Sprintf("%s%s::text", columnPrefix, v)
	}), ",")
	b := strings.Join(lo.Map(columns, func(v string, i int) string {
		return fmt.Sprintf(`split_part(n.model_keys, '%s', %d) AS %s`, sep, i+1, v)
	}), ",\n")

	tableName := tablePrefix + ParseTableNameWithDB(db, &ActivityLog{})

	return fmt.Sprintf(`
	(%s) IN (
	    WITH NoteRecords AS (
		    SELECT model_name, model_keys, created_at, creator_id
		    FROM %s
		    WHERE action = '%s' AND deleted_at IS NULL
		        AND model_name = '%s'
		),
		LastViewedAts AS (
		    SELECT model_name, model_keys, MAX(updated_at) AS last_viewed_at
		    FROM %s
		    WHERE action = '%s' AND creator_id = '%s' AND deleted_at IS NULL
		        AND model_name = '%s'
		    GROUP BY model_name, model_keys
		)
		
	    SELECT
%s
	    FROM NoteRecords n
	    LEFT JOIN LastViewedAts lva
	        ON n.model_name = lva.model_name
	        AND n.model_keys = lva.model_keys
	    WHERE n.creator_id <> '%s' 
	        AND (lva.last_viewed_at IS NULL OR n.created_at > lva.last_viewed_at)
	    GROUP BY n.model_keys
    )`, a, tableName, ActionNote, modelName, tableName, ActionLastView, creatorID, modelName, b, creatorID)
}

func (ab *Builder) GetNotesCounts(ctx context.Context, modelName string, modelKeyses []string) ([]*NoteCount, error) {
	user, err := ab.currentUserFunc(ctx)
	if err != nil {
		return nil, err
	}
	return getNotesCounts(ab.db, ab.tablePrefix, user.ID, modelName, modelKeyses)
}

func (ab *Builder) MarkAllNotesAsRead(ctx context.Context) error {
	user, err := ab.currentUserFunc(ctx)
	if err != nil {
		return err
	}
	return markAllNotesAsRead(ab.db, ab.tablePrefix, user.ID)
}

func (amb *ModelBuilder) SQLConditionHasUnreadNotes(creatorID string, columnPrefix string) string {
	return sqlConditionHasUnreadNotes(amb.ab.db, amb.ab.tablePrefix, creatorID, ParseModelName(amb.ref), amb.keyColumns, ModelKeysSeparator, columnPrefix)
}
