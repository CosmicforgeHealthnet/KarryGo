ALTER TABLE commerce_delivery_requirement_snapshots
    ADD CONSTRAINT commerce_delivery_requirement_snapshots_no_car_execution_chk
        CHECK (selected_vehicle_class <> 'CAR'),
    DROP CONSTRAINT commerce_delivery_requirement_snapshots_selected_service_check,
    ADD CONSTRAINT commerce_delivery_requirement_snapshots_selected_service_check
        CHECK (selected_service IN ('','DISPATCH','HAULING','VENDOR_OWN_DELIVERY')),
    DROP CONSTRAINT commerce_delivery_requirement_snap_selected_vehicle_class_check,
    ADD CONSTRAINT commerce_delivery_requirement_snap_selected_vehicle_class_check
        CHECK (selected_vehicle_class IN ('','MOTORCYCLE','VAN','TRUCK'));

ALTER TABLE commerce_fulfilment_plan_groups
    DROP CONSTRAINT commerce_fulfilment_plan_groups_vehicle_class_check,
    ADD CONSTRAINT commerce_fulfilment_plan_groups_vehicle_class_check
        CHECK (vehicle_class IN ('','MOTORCYCLE','VAN','TRUCK'));

ALTER TABLE commerce_fulfilment_missions
    DROP CONSTRAINT commerce_fulfilment_missions_vehicle_class_check,
    ADD CONSTRAINT commerce_fulfilment_missions_vehicle_class_check
        CHECK (vehicle_class IN ('','MOTORCYCLE','VAN','TRUCK')),
    DROP CONSTRAINT commerce_fulfilment_missions_service_check,
    ADD CONSTRAINT commerce_fulfilment_missions_service_check
        CHECK (service IN ('DISPATCH','HAULING','VENDOR_OWN_DELIVERY'));
