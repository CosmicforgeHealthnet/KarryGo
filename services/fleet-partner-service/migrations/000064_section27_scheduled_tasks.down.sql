DELETE FROM fleet_partner_role_permissions WHERE permission_code IN
    ('scheduled_tasks.read','scheduled_tasks.manage','scheduled_tasks.run',
     'worker_operations.read','worker_operations.manage');
DELETE FROM fleet_partner_permissions WHERE code IN
    ('scheduled_tasks.read','scheduled_tasks.manage','scheduled_tasks.run',
     'worker_operations.read','worker_operations.manage');

DROP TRIGGER IF EXISTS trg_fleet_scheduled_task_runs_append_only ON fleet_partner_scheduled_task_runs;
DROP TABLE IF EXISTS fleet_partner_scheduled_task_runs;
DROP TABLE IF EXISTS fleet_partner_scheduled_tasks;
