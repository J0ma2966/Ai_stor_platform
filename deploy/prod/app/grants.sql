-- Права роли API (boutique_app). Выполняется владельцем базы после каждой миграции
-- (deploy.sh app), идемпотентно. Каждая таблица и последовательность, созданная миграциями
-- (владелец boutique), получает чтение и запись; журналы token_ledger и audit_log — только
-- SELECT и INSERT. Даже при уязвимости в коде роль API не может изменить или удалить запись
-- журнала, отключить триггер или поменять схему.
\set ON_ERROR_STOP on
GRANT USAGE ON SCHEMA public TO boutique_app;
DO $$
DECLARE r record;
BEGIN
  FOR r IN SELECT c.relname, c.relkind FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
           WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p', 'v', 'm', 'S')
             AND pg_get_userbyid(c.relowner) = current_user LOOP
    IF r.relkind = 'S' THEN
      EXECUTE format('GRANT USAGE, SELECT, UPDATE ON SEQUENCE public.%I TO boutique_app', r.relname);
    ELSIF r.relname = 'goose_db_version' THEN
      EXECUTE format('REVOKE ALL ON TABLE public.%I FROM boutique_app', r.relname);
      EXECUTE format('GRANT SELECT ON TABLE public.%I TO boutique_app', r.relname);
    ELSIF r.relname IN ('token_ledger', 'audit_log') THEN
      EXECUTE format('GRANT SELECT, INSERT ON TABLE public.%I TO boutique_app', r.relname);
      EXECUTE format('REVOKE UPDATE, DELETE, TRUNCATE ON TABLE public.%I FROM boutique_app', r.relname);
    ELSE
      EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.%I TO boutique_app', r.relname);
    END IF;
  END LOOP;
END $$;
