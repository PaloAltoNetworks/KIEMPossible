CREATE DATABASE IF NOT EXISTS clusterlogo;
USE clusterlogo;

CREATE TABLE IF NOT EXISTS permission (
    id INT AUTO_INCREMENT PRIMARY KEY,
    entity_name VARCHAR(255) NOT NULL,
    entity_type VARCHAR(255) NOT NULL,
    api_group VARCHAR(255) NOT NULL,
    resource_type VARCHAR(255) NOT NULL,
    verb VARCHAR(255) NOT NULL,
    permission_scope VARCHAR(255) NOT NULL,
    last_used_time DATETIME NULL,
    last_used_resource VARCHAR(255) NULL,
    UNIQUE KEY unique_permission (entity_name, entity_type, api_group, resource_type, verb, permission_scope)
);
