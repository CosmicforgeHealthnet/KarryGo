const jobTransitions = <String, List<String>>{
  'PLANNED': ['QUEUED', 'IN_PROGRESS', 'CANCELLED'],
  'QUEUED': ['IN_PROGRESS', 'BLOCKED', 'CANCELLED'],
  'IN_PROGRESS': ['BLOCKED', 'READY', 'CANCELLED'],
  'BLOCKED': ['IN_PROGRESS', 'CANCELLED'],
  'READY': ['COMPLETED'],
};
const stationTransitions = <String, List<String>>{
  'PENDING': ['READY', 'CANCELLED'],
  'READY': ['IN_PROGRESS', 'SKIPPED', 'CANCELLED'],
  'IN_PROGRESS': ['BLOCKED', 'COMPLETED', 'CANCELLED'],
  'BLOCKED': ['IN_PROGRESS', 'CANCELLED'],
};
