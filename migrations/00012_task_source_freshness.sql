-- +goose Up
-- Keep provenance in the existing source JSONB. Never backfill a publication
-- date from created_at: that would make an old import appear to be recent.
-- Provider IDs (notably РЕШУ) are local to each subject's site.
DROP INDEX idx_tasks_source;
CREATE UNIQUE INDEX idx_tasks_source
    ON tasks (subject_id, (source ->> 'provider'), (source ->> 'extern_id'))
    WHERE source IS NOT NULL;
-- +goose StatementBegin
CREATE FUNCTION task_source_current(src jsonb, at_time timestamptz) RETURNS boolean
LANGUAGE plpgsql STABLE AS $$
DECLARE
    published timestamptz;
    verified timestamptz;
    allowed text;
BEGIN
    allowed := CASE src->>'provider'
        WHEN 'fipi' THEN '(fipi\.ru|doc\.fipi\.ru|ege\.fipi\.ru)'
        WHEN 'openfipi' THEN '(openfipi\.devinf\.ru|fipi\.ru|doc\.fipi\.ru|ege\.fipi\.ru)'
        WHEN 'sdamgia' THEN '((math|rus|inf|soc)(-ege|\.ege)\.sdamgia\.ru|ege\.sdamgia\.ru|fipi\.ru|doc\.fipi\.ru|ege\.fipi\.ru)'
        ELSE NULL END;
    IF allowed IS NULL OR coalesce(btrim(src->>'extern_id'), '') = ''
       OR coalesce(btrim(src->>'date_evidence'), '') = ''
       OR NOT coalesce(src->>'url' ~ ('^https://' || allowed || '(/|$)'), false)
       OR NOT coalesce(src->>'date_evidence_url' ~ ('^https://' || allowed || '(/|$)'), false) THEN
        RETURN false;
    END IF;
    published := (src->>'published_at')::timestamptz;
    verified := (src->>'verified_at')::timestamptz;
    RETURN coalesce(published >= at_time - interval '1 year' AND published <= at_time
        AND verified >= published AND verified <= at_time, false);
EXCEPTION WHEN invalid_datetime_format OR datetime_field_overflow THEN
    RETURN false;
END;
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION task_source_current(jsonb, timestamptz);
DROP INDEX idx_tasks_source;
CREATE UNIQUE INDEX idx_tasks_source
    ON tasks ((source ->> 'provider'), (source ->> 'extern_id'))
    WHERE source IS NOT NULL;
