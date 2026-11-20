-- MySQL dump 10.13  Distrib 8.0.39, for Linux (x86_64)
-- Host: localhost    Database: pellucid_supply
-- Fictional demo data for brainDB

/*!40101 SET NAMES utf8mb4 */;
SET FOREIGN_KEY_CHECKS=0;

DROP TABLE IF EXISTS `suppliers`;
CREATE TABLE `suppliers` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(120) NOT NULL,
  `city` varchar(80) DEFAULT NULL,
  `country` char(2) NOT NULL,
  `notes` text,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

DROP TABLE IF EXISTS `products`;
CREATE TABLE `products` (
  `sku` varchar(16) NOT NULL,
  `name` varchar(160) NOT NULL,
  `category` varchar(32) NOT NULL,
  `supplier_id` int DEFAULT NULL,
  `unit_cost_eur` decimal(10,2) NOT NULL,
  `lead_time_days` int DEFAULT NULL,
  PRIMARY KEY (`sku`),
  KEY `fk_products_supplier` (`supplier_id`),
  CONSTRAINT `fk_products_supplier` FOREIGN KEY (`supplier_id`) REFERENCES `suppliers` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

LOCK TABLES `suppliers` WRITE;
INSERT INTO `suppliers` VALUES (1,'Pellucid Manufacturing S.r.l.','Milan','IT','Ships espresso machines in 4\'s per pallet'),(2,'Autoflow GmbH','Stuttgart','DE','Bean-to-cup specialist; 6-week lead time'),(3,'Grindwell Ltd','King\'s Lynn','GB','Burrs \"made in-house\"; 2 weeks lead time'),(4,'ClearFlow Water AB','Lund','SE','Filter cartridges, monthly deliveries');
UNLOCK TABLES;

LOCK TABLES `products` WRITE;
INSERT INTO `products` VALUES ('PX-200','Pellucid PX-200 two-group espresso machine','espresso',1,4345.0,21),('PX-300','Pellucid PX-300 three-group espresso machine','espresso',1,5995.0,21),('AX-1','Autoflow AX-1 bean-to-cup machine','bean-to-cup',2,2970.0,42),('GR-64','Grindwell GR-64 flat burr grinder (\"64 mm\")','grinder',3,797.5,14),('GR-83','Grindwell GR-83 flat burr grinder','grinder',3,1292.5,14),('WF-10','ClearFlow WF-10 water filter cartridge','consumable',4,77.0,7),('SV-CARE','CareService annual maintenance plan','service',NULL,360.0,NULL),('SV-PRIO','CareService Priority plan (4 h response)','service',NULL,780.0,NULL);
UNLOCK TABLES;
SET FOREIGN_KEY_CHECKS=1;
-- Dump completed on 2026-11-18 22:04:11
