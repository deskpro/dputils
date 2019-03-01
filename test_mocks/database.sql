CREATE TABLE IF NOT EXISTS `agent_activity` (
  `date_active` datetime NOT NULL,
  `agent_id` int(11) NOT NULL,
  PRIMARY KEY (`date_active`,`agent_id`),
  KEY `IDX_9AA510CE3414710B` (`agent_id`),
  KEY `date_created_idx` (`date_active`),
  CONSTRAINT `FK_9AA510CE3414710B` FOREIGN KEY (`agent_id`) REFERENCES `people` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
