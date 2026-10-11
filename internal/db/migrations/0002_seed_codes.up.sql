-- SPDX-License-Identifier: AGPL-3.0-only
--
-- Canonical quantity codes: LOINC governs the measurement, UCUM the unit, storage_unit
-- is the literal unit ohs_data.quantity_samples.unit carries. The first block is the
-- curated mapping; the second registers the storage unit of every other declared type.
INSERT INTO ohs_data.quantity_codes
    (canonical_code, loinc_code, snomed_code, ucum_unit, storage_unit, value_type, hot_table, promoted)
VALUES
    ('heart_rate', '8867-4', '364075005', '/min', 'bpm', 'num', 'ohs_data.quantity_samples', true),
    ('resting_heart_rate', '40443-4', NULL, '/min', 'bpm', 'num', 'ohs_data.quantity_samples', true),
    ('walking_heart_rate_average', NULL, NULL, '/min', 'bpm', 'num', 'ohs_data.quantity_samples', true),
    ('hrv_sdnn', '80404-7', NULL, 'ms', 'ms', 'num', 'ohs_data.quantity_samples', true),
    ('vo2_max', NULL, NULL, 'mL/kg/min', 'mL/kg/min', 'num', 'ohs_data.quantity_samples', true),
    ('oxygen_saturation', '59408-5', '431314004', '%', 'percent', 'num', 'ohs_data.quantity_samples', true),
    ('respiratory_rate', '9279-1', '86290005', '/min', 'bpm', 'num', 'ohs_data.quantity_samples', true),
    ('body_temperature', '8310-5', '386725007', 'Cel', 'celsius', 'num', 'ohs_data.quantity_samples', true),
    ('blood_pressure_systolic', '8480-6', '271649006', 'mm[Hg]', 'mmHg', 'num', 'ohs_data.quantity_samples', true),
    ('blood_pressure_diastolic', '8462-4', '271650006', 'mm[Hg]', 'mmHg', 'num', 'ohs_data.quantity_samples', true),
    ('blood_glucose', '2339-0', '434912009', 'mg/dL', 'mg/dL', 'num', 'ohs_data.quantity_samples', true),
    ('body_mass', '29463-7', '27113001', 'kg', 'kg', 'num', 'ohs_data.quantity_samples', true),
    ('body_mass_index', '39156-5', '60621009', 'kg/m2', 'kg/m2', 'num', 'ohs_data.quantity_samples', true),
    ('body_fat_percentage', '41982-0', NULL, '%', 'percent', 'num', 'ohs_data.quantity_samples', true),
    ('body_fat_mass', NULL, NULL, 'kg', 'kg', 'num', 'ohs_data.quantity_samples', true),
    ('water_mass', NULL, NULL, 'kg', 'kg', 'num', 'ohs_data.quantity_samples', true),
    ('height', '8302-2', '50373000', 'cm', 'cm', 'num', 'ohs_data.quantity_samples', true),
    ('step_count', '55423-8', NULL, '{steps}', 'count', 'num', 'ohs_data.quantity_samples', true),
    ('distance_walking_running', NULL, NULL, 'm', 'm', 'num', 'ohs_data.quantity_samples', true),
    ('active_energy_burned', NULL, NULL, 'kcal', 'kcal', 'num', 'ohs_data.quantity_samples', true),
    ('basal_energy_burned', NULL, NULL, 'kcal', 'kcal', 'num', 'ohs_data.quantity_samples', true),
    ('flights_climbed', NULL, NULL, '{flights}', 'count', 'num', 'ohs_data.quantity_samples', true),
    ('sleep_analysis', NULL, '258158006', 's', NULL, 'code', 'ohs_data.sleep_sessions', true),
    ('active_energy_burned_intraday', NULL, NULL, NULL, 'kcal', 'num', 'ohs_data.quantity_samples', true),
    ('apple_exercise_time', NULL, NULL, NULL, 'min', 'num', 'ohs_data.quantity_samples', true),
    ('apple_move_time', NULL, NULL, NULL, 'min', 'num', 'ohs_data.quantity_samples', true),
    ('apple_stand_time', NULL, NULL, NULL, 'min', 'num', 'ohs_data.quantity_samples', true),
    ('body_temperature_deviation', NULL, NULL, NULL, 'celsius', 'num', 'ohs_data.quantity_samples', true),
    ('bone_mass', NULL, NULL, NULL, 'kg', 'num', 'ohs_data.quantity_samples', true),
    ('distance_walking_running_intraday', NULL, NULL, NULL, 'm', 'num', 'ohs_data.quantity_samples', true),
    ('lean_body_mass', NULL, NULL, NULL, 'kg', 'num', 'ohs_data.quantity_samples', true),
    ('muscle_mass', NULL, NULL, NULL, 'kg', 'num', 'ohs_data.quantity_samples', true),
    ('pulse_wave_velocity', NULL, NULL, NULL, 'm/s', 'num', 'ohs_data.quantity_samples', true),
    ('readiness_hrv_balance', NULL, NULL, NULL, 'index', 'num', 'ohs_data.quantity_samples', true),
    ('readiness_score', NULL, NULL, NULL, 'score', 'num', 'ohs_data.quantity_samples', true),
    ('skin_temperature', NULL, NULL, NULL, 'celsius', 'num', 'ohs_data.quantity_samples', true),
    ('step_count_intraday', NULL, NULL, NULL, 'count', 'num', 'ohs_data.quantity_samples', true),
    ('strain', NULL, NULL, NULL, 'index', 'num', 'ohs_data.quantity_samples', true),
    ('vascular_age', NULL, NULL, NULL, 'years', 'num', 'ohs_data.quantity_samples', true),
    ('visceral_fat_rating', NULL, NULL, NULL, 'index', 'num', 'ohs_data.quantity_samples', true),
    ('workout_strain', NULL, NULL, NULL, 'index', 'num', 'ohs_data.quantity_samples', true)
ON CONFLICT (canonical_code) DO UPDATE SET
    loinc_code   = EXCLUDED.loinc_code,
    snomed_code  = EXCLUDED.snomed_code,
    ucum_unit    = EXCLUDED.ucum_unit,
    storage_unit = EXCLUDED.storage_unit,
    value_type   = EXCLUDED.value_type,
    hot_table    = EXCLUDED.hot_table,
    promoted     = EXCLUDED.promoted,
    updated_at   = now();
