CREATE DATABASE IF NOT EXISTS `inventory` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'inventory'@'%' IDENTIFIED BY 'inventory';
GRANT ALL PRIVILEGES ON `inventory`.* TO 'inventory'@'%';
