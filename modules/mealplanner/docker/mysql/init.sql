CREATE DATABASE IF NOT EXISTS `mealplanner` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'mealplanner'@'%' IDENTIFIED BY 'mealplanner';
GRANT ALL PRIVILEGES ON `mealplanner`.* TO 'mealplanner'@'%';
