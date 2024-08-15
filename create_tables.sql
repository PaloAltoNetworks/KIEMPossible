CREATE DATABASE IF NOT EXISTS clusterlogo;

CREATE TABLE IF NOT EXISTS clusterlogo.permission (
    id INT AUTO_INCREMENT PRIMARY KEY,
    entity_name VARCHAR(100) NOT NULL,
    entity_type VARCHAR(30) NOT NULL,
    api_group VARCHAR(150) NOT NULL,
    resource_type VARCHAR(100) NOT NULL,
    verb VARCHAR(30) NOT NULL,
    permission_scope VARCHAR(70) NOT NULL,
    permission_source VARCHAR(100) NOT NULL,
    permission_source_type VARCHAR(20) NOT NULL,
    permission_binding VARCHAR(100) NOT NULL,
    permission_binding_type VARCHAR(20) NOT NULL,
    last_used_time DATETIME NULL,
    last_used_resource VARCHAR(150) NULL,
    UNIQUE KEY unique_permission (entity_name, entity_type, api_group, resource_type, verb, permission_scope, permission_source, permission_source_type, permission_binding, permission_binding_type)
);




