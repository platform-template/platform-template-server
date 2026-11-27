CREATE DATABASE IF NOT EXISTS `recipes` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'recipes'@'%' IDENTIFIED BY 'recipes';
GRANT ALL PRIVILEGES ON `recipes`.* TO 'recipes'@'%';
