const config = {
  _id: "rs0",
  members: [
    { _id: 0, host: "mongo1:27017", priority: 2 },
    { _id: 1, host: "mongo2:27017", priority: 1 },
    { _id: 2, host: "mongo3:27017", priority: 1 },
  ],
};

try {
  rs.status();
  print("Replica set rs0 is already initialized.");
} catch (error) {
  if (error.codeName !== "NotYetInitialized") {
    throw error;
  }

  rs.initiate(config);
  print("Replica set rs0 initialized.");
}

const deadline = Date.now() + 60 * 1000;
while (Date.now() < deadline) {
  try {
    const status = rs.status();
    const primary = status.members.find((member) => member.stateStr === "PRIMARY");
    const healthyMembers = status.members.filter((member) => member.health === 1);

    if (primary && healthyMembers.length === config.members.length) {
      print(`Replica set is ready. Primary: ${primary.name}`);
      quit(0);
    }
  } catch (error) {
    // The replica set can take a few seconds to elect a primary.
  }

  sleep(1000);
}

print("Timed out waiting for replica set election.");
quit(1);
