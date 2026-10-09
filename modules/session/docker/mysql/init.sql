CREATE DATABASE IF NOT EXISTS `session` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'session'@'%' IDENTIFIED BY 'session';
GRANT ALL PRIVILEGES ON `session`.* TO 'session'@'%';
