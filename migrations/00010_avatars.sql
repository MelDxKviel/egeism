-- +goose Up
-- +goose StatementBegin
-- Профильный аватар (ученики, учителя, админы). JSONB-конфиг с дискриминатором
-- kind: сегодня только "builder" — параметрический SVG, который клиенты
-- собирают сами из индексов частей (причёска/глаза/рот/аксессуар) и цветов;
-- "photo" зарезервирован под будущие загружаемые фото (photo_key = ключ в
-- MinIO), чтобы переход не требовал новой миграции. NULL = пользователь ничего
-- не выбирал — клиенты рисуют детерминированный дефолт из id, так что аватар
-- есть у всех сразу и бэкфилл не нужен.
ALTER TABLE users ADD COLUMN avatar jsonb;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users DROP COLUMN IF EXISTS avatar;
-- +goose StatementEnd
