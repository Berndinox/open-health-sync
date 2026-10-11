-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Storage units of the Apple Health quantity types declared in internal/model/units.go
-- with the Apple Health connector, so quantity_codes keeps registering every declared type.
INSERT INTO ohs_data.quantity_codes
    (canonical_code, loinc_code, snomed_code, ucum_unit, storage_unit, value_type, hot_table, promoted)
VALUES
    ('apple_sleeping_wrist_temperature', NULL, NULL, NULL, 'celsius', 'num', 'ohs_data.quantity_samples', true),
    ('basal_body_temperature', NULL, NULL, NULL, 'celsius', 'num', 'ohs_data.quantity_samples', true),
    ('blood_alcohol_content', NULL, NULL, NULL, 'percent', 'num', 'ohs_data.quantity_samples', true),
    ('cycling_power', NULL, NULL, NULL, 'W', 'num', 'ohs_data.quantity_samples', true),
    ('distance_cycling', NULL, NULL, NULL, 'm', 'num', 'ohs_data.quantity_samples', true),
    ('distance_downhill_snow_sports', NULL, NULL, NULL, 'm', 'num', 'ohs_data.quantity_samples', true),
    ('distance_swimming', NULL, NULL, NULL, 'm', 'num', 'ohs_data.quantity_samples', true),
    ('distance_wheelchair', NULL, NULL, NULL, 'm', 'num', 'ohs_data.quantity_samples', true),
    ('electrodermal_activity', NULL, NULL, NULL, 'uS', 'num', 'ohs_data.quantity_samples', true),
    ('environmental_audio_exposure', NULL, NULL, NULL, 'dBASPL', 'num', 'ohs_data.quantity_samples', true),
    ('environmental_sound_reduction', NULL, NULL, NULL, 'dBASPL', 'num', 'ohs_data.quantity_samples', true),
    ('forced_expiratory_volume_1', NULL, NULL, NULL, 'L', 'num', 'ohs_data.quantity_samples', true),
    ('forced_vital_capacity', NULL, NULL, NULL, 'L', 'num', 'ohs_data.quantity_samples', true),
    ('headphone_audio_exposure', NULL, NULL, NULL, 'dBASPL', 'num', 'ohs_data.quantity_samples', true),
    ('inhaler_usage', NULL, NULL, NULL, 'count', 'num', 'ohs_data.quantity_samples', true),
    ('insulin_delivery', NULL, NULL, NULL, 'IU', 'num', 'ohs_data.quantity_samples', true),
    ('nike_fuel', NULL, NULL, NULL, 'count', 'num', 'ohs_data.quantity_samples', true),
    ('number_of_alcoholic_beverages', NULL, NULL, NULL, 'count', 'num', 'ohs_data.quantity_samples', true),
    ('number_of_times_fallen', NULL, NULL, NULL, 'count', 'num', 'ohs_data.quantity_samples', true),
    ('peak_expiratory_flow_rate', NULL, NULL, NULL, 'L/min', 'num', 'ohs_data.quantity_samples', true),
    ('physical_effort', NULL, NULL, NULL, 'kcal/kg/h', 'num', 'ohs_data.quantity_samples', true),
    ('push_count', NULL, NULL, NULL, 'count', 'num', 'ohs_data.quantity_samples', true),
    ('running_ground_contact_time', NULL, NULL, NULL, 'ms', 'num', 'ohs_data.quantity_samples', true),
    ('running_power', NULL, NULL, NULL, 'W', 'num', 'ohs_data.quantity_samples', true),
    ('running_speed', NULL, NULL, NULL, 'm/s', 'num', 'ohs_data.quantity_samples', true),
    ('running_stride_length', NULL, NULL, NULL, 'm', 'num', 'ohs_data.quantity_samples', true),
    ('running_vertical_oscillation', NULL, NULL, NULL, 'cm', 'num', 'ohs_data.quantity_samples', true),
    ('underwater_depth', NULL, NULL, NULL, 'm', 'num', 'ohs_data.quantity_samples', true),
    ('uv_exposure', NULL, NULL, NULL, 'index', 'num', 'ohs_data.quantity_samples', true),
    ('waist_circumference', NULL, NULL, NULL, 'cm', 'num', 'ohs_data.quantity_samples', true),
    ('walking_speed', NULL, NULL, NULL, 'm/s', 'num', 'ohs_data.quantity_samples', true),
    ('water_temperature', NULL, NULL, NULL, 'celsius', 'num', 'ohs_data.quantity_samples', true)
ON CONFLICT (canonical_code) DO UPDATE SET
    storage_unit = EXCLUDED.storage_unit,
    value_type   = EXCLUDED.value_type,
    hot_table    = EXCLUDED.hot_table,
    promoted     = EXCLUDED.promoted,
    updated_at   = now();
