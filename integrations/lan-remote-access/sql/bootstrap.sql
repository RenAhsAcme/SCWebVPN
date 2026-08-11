DO $$
DECLARE
    webvpn_entity_id integer;
    windows_connection_id integer;
BEGIN
    UPDATE guacamole_entity
       SET name = 'webvpn'
     WHERE name = 'tailnet'
       AND type = 'USER'
       AND NOT EXISTS (
           SELECT 1 FROM guacamole_entity WHERE name = 'webvpn' AND type = 'USER'
       );

    SELECT entity_id INTO webvpn_entity_id
      FROM guacamole_entity
     WHERE name = 'webvpn' AND type = 'USER';

    IF webvpn_entity_id IS NULL THEN
        INSERT INTO guacamole_entity (name, type)
        VALUES ('webvpn', 'USER')
        RETURNING entity_id INTO webvpn_entity_id;

        INSERT INTO guacamole_user (
            entity_id,
            password_hash,
            password_salt,
            password_date
        ) VALUES (
            webvpn_entity_id,
            decode(md5(random()::text) || md5(random()::text), 'hex'),
            decode(md5(random()::text) || md5(random()::text), 'hex'),
            CURRENT_TIMESTAMP
        );
    END IF;

    INSERT INTO guacamole_system_permission (entity_id, permission)
    SELECT webvpn_entity_id, 'ADMINISTER'
    WHERE NOT EXISTS (
        SELECT 1 FROM guacamole_system_permission
         WHERE entity_id = webvpn_entity_id AND permission = 'ADMINISTER'
    );

    SELECT connection_id INTO windows_connection_id
      FROM guacamole_connection
     WHERE connection_name = 'Windows PC' AND parent_id IS NULL;

    IF windows_connection_id IS NULL THEN
        INSERT INTO guacamole_connection (
            connection_name,
            protocol,
            max_connections,
            max_connections_per_user
        ) VALUES ('Windows PC', 'rdp', 1, 1)
        RETURNING connection_id INTO windows_connection_id;
    END IF;

    INSERT INTO guacamole_connection_permission (entity_id, connection_id, permission)
    SELECT webvpn_entity_id, windows_connection_id, 'READ'
    WHERE NOT EXISTS (
        SELECT 1 FROM guacamole_connection_permission
         WHERE entity_id = webvpn_entity_id
           AND connection_id = windows_connection_id
           AND permission = 'READ'
    );

    DELETE FROM guacamole_connection_parameter
     WHERE connection_id = windows_connection_id
       AND parameter_name IN ('enable-audio', 'color-depth', 'console-audio');

    INSERT INTO guacamole_connection_parameter (connection_id, parameter_name, parameter_value)
    VALUES
        (windows_connection_id, 'hostname', '__PC_LAN_ADDRESS__'),
        (windows_connection_id, 'port', '3389'),
        (windows_connection_id, 'security', 'nla'),
        (windows_connection_id, 'ignore-cert', 'true'),
        (windows_connection_id, 'console', 'false'),
        (windows_connection_id, 'resize-method', 'display-update'),
        (windows_connection_id, 'disable-audio', 'false'),
        (windows_connection_id, 'enable-audio-input', 'true'),
        (windows_connection_id, 'enable-drive', 'true'),
        (windows_connection_id, 'drive-name', 'Remote Files'),
        (windows_connection_id, 'drive-path', '/drive'),
        (windows_connection_id, 'create-drive-path', 'true'),
        (windows_connection_id, 'enable-wallpaper', 'false'),
        (windows_connection_id, 'enable-font-smoothing', 'false')
    ON CONFLICT (connection_id, parameter_name)
    DO UPDATE SET parameter_value = EXCLUDED.parameter_value;
END $$;
