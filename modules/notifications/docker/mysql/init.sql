CREATE DATABASE IF NOT EXISTS `notifications` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'notifications'@'%' IDENTIFIED BY 'notifications';
GRANT ALL PRIVILEGES ON `notifications`.* TO 'notifications'@'%';
