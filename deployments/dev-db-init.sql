-- Development-only bootstrap for the disposable Compose MariaDB.
-- Production databases/users are created by an operator (docs/RUNBOOK.md),
-- with real secrets that never live in Git.

CREATE DATABASE IF NOT EXISTS pawn_ink CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE DATABASE IF NOT EXISTS pawn_ink_collar CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- Runtime account: CRUD only on the two Pawn.Ink databases.
CREATE USER IF NOT EXISTS 'pawn_app'@'%' IDENTIFIED BY 'pawn-app-dev';
GRANT SELECT, INSERT, UPDATE, DELETE ON pawn_ink.* TO 'pawn_app'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON pawn_ink_collar.* TO 'pawn_app'@'%';

-- Migration account: owns DDL on the two databases.
CREATE USER IF NOT EXISTS 'pawn_migrator'@'%' IDENTIFIED BY 'pawn-migrator-dev';
GRANT ALL PRIVILEGES ON pawn_ink.* TO 'pawn_migrator'@'%';
GRANT ALL PRIVILEGES ON pawn_ink_collar.* TO 'pawn_migrator'@'%';

-- Integration tests create and drop randomly named pi_test_* databases.
CREATE USER IF NOT EXISTS 'pawn_test'@'%' IDENTIFIED BY 'pawn-test-dev';
GRANT ALL PRIVILEGES ON `pi\_test\_%`.* TO 'pawn_test'@'%';

FLUSH PRIVILEGES;
