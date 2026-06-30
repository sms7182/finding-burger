
INSERT INTO vendors (name, service_zone, base_load, current_load) VALUES
('Spree Kitchen',
 ST_GeomFromText('POLYGON((13.38 52.50, 13.42 52.50, 13.42 52.54, 13.38 52.54, 13.38 52.50))', 4326),
 2, 2),
('Mitte Meals',
 ST_GeomFromText('POLYGON((13.40 52.51, 13.44 52.51, 13.44 52.55, 13.40 52.55, 13.40 52.51))', 4326),
 5, 5),
('Kreuzberg Eats',
 ST_GeomFromText('POLYGON((13.40 52.48, 13.44 52.48, 13.44 52.51, 13.40 52.51, 13.40 52.48))', 4326),
 1, 1);
