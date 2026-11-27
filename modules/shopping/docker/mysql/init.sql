CREATE DATABASE IF NOT EXISTS `shopping` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'shopping'@'%' IDENTIFIED BY 'shopping';
GRANT ALL PRIVILEGES ON `shopping`.* TO 'shopping'@'%';
