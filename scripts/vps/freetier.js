// What Google's free tier covers, and the checks that keep every gcloud command inside it.
// https://docs.cloud.google.com/free/docs/free-cloud-features (Compute Engine row)

export const FREE_MACHINE_TYPE = 'e2-micro';
export const FREE_REGIONS = ['us-west1', 'us-central1', 'us-east1'];
export const FREE_DISK_TYPE = 'pd-standard';
export const FREE_DISK_GB = 30;
export const STANDARD_PROVISIONING = 'STANDARD';
export const NETWORK = 'money24-net';
export const SUBNET = 'money24-subnet';
export const SUBNET_RANGE = '10.24.0.0/24';
export const FIREWALL_RULE = 'money24-allow-iap-ssh';
export const IAP_RANGE = '35.235.240.0/20'; // Google's IAP TCP forwarding source range
export const VM_ACCOUNT = 'money24-vm';
export const IMAGE_FAMILY = 'debian-12';
export const IMAGE_PROJECT = 'debian-cloud';
const ZONE_SUFFIX = /^[a-z]$/;
const BOOT_DISK_SIZE = /^(\d+)GB$/;
const SIZE_GB = /^\d+$/;

// Every gcloud command the kit runs. Anything else is refused before it starts.
const ALLOWED_COMMANDS = [
  '--version', 'auth print-access-token', 'services enable',
  'pubsub topics describe', 'pubsub topics create',
  'iam service-accounts describe', 'iam service-accounts create',
  'projects add-iam-policy-binding', 'billing accounts add-iam-policy-binding', 'billing projects describe',
  'billing budgets list', 'billing budgets create', 'billing budgets update',
  'functions deploy', 'functions describe', 'functions add-invoker-policy-binding',
  'artifacts repositories set-cleanup-policies',
  'compute networks describe', 'compute networks create',
  'compute networks subnets describe', 'compute networks subnets create',
  'compute firewall-rules describe', 'compute firewall-rules create',
  'compute instances describe', 'compute instances create', 'compute instances list',
  'compute disks describe', 'compute disks list', 'compute snapshots list', 'compute addresses list',
  'compute forwarding-rules list', 'compute routers list', 'compute images list',
  'compute ssh', 'compute scp',
];
// Billable resource groups: the kit may only list or describe them, never create or change one.
const BILLABLE_GROUPS = new Set([
  'snapshots', 'addresses', 'forwarding-rules', 'target-pools', 'target-instances', 'target-http-proxies',
  'target-https-proxies', 'target-tcp-proxies', 'target-ssl-proxies', 'target-grpc-proxies',
  'backend-services', 'backend-buckets', 'url-maps', 'health-checks', 'routers', 'images', 'machine-images',
  'instance-templates', 'instance-groups', 'resource-policies', 'reservations', 'commitments', 'disks',
  'ssl-certificates', 'security-policies', 'vpn-gateways', 'vpn-tunnels', 'interconnects', 'tpus', 'sql',
]);
const READ_VERBS = new Set(['list', 'describe']);
// Flags that add a disk, an IPv4 address, a GPU, or a non-standard machine. Refused on any command.
const BILLABLE_FLAGS = [
  '--address', '--create-disk', '--disk', '--local-ssd', '--accelerator', '--preemptible',
  '--network-tier', '--reservation', '--reservation-affinity', '--min-cpu-platform', '--machine-image',
  '--source-snapshot', '--source-machine-image', '--instance-termination-action', '--max-run-duration',
];

/** Splits `--name=value` flags from the leading command words. */
function parse(args) {
  const words = [];
  const flags = new Map();
  for (const arg of args) {
    if (!arg.startsWith('--')) {
      if (flags.size === 0) words.push(arg);
      continue;
    }
    const cut = arg.indexOf('=');
    flags.set(cut < 0 ? arg : arg.slice(0, cut), cut < 0 ? '' : arg.slice(cut + 1));
  }
  return { words, flags };
}

export function freeZone(zone) {
  const cut = zone.lastIndexOf('-');
  return cut > 0 && FREE_REGIONS.includes(zone.slice(0, cut)) && ZONE_SUFFIX.test(zone.slice(cut + 1));
}

/** Refuses an instance create that would leave the free tier. */
export function assertFreeInstance(args) {
  const { flags } = parse(args);
  const size = BOOT_DISK_SIZE.exec(flags.get('--boot-disk-size') ?? '');
  const nic = (flags.get('--network-interface') ?? '').split(',');
  const problems = [
    flags.get('--machine-type') !== FREE_MACHINE_TYPE && `machine type must be ${FREE_MACHINE_TYPE}`,
    !freeZone(flags.get('--zone') ?? '') && `zone must be in ${FREE_REGIONS.join(', ')}`,
    flags.get('--boot-disk-type') !== FREE_DISK_TYPE && `boot disk must be ${FREE_DISK_TYPE}`,
    (!size || Number(size[1]) > FREE_DISK_GB) && `boot disk must be at most ${FREE_DISK_GB}GB`,
    flags.get('--provisioning-model') !== STANDARD_PROVISIONING && 'provisioning model must be STANDARD',
    (!nic.includes('no-address') || nic.some((part) => part.startsWith('address='))) &&
      'the network interface must have no external IPv4 (Google bills it hourly)',
  ].filter(Boolean);
  if (problems.length) throw new Error(`Refusing a billable instance: ${problems.join('; ')}.`);
}

/** Refuses any gcloud command the kit does not know, and any that would create something billable. */
export function assertAllowed(args) {
  const { words, flags } = parse(args);
  const command = words.join(' ');
  const known = ALLOWED_COMMANDS.some((allowed) => command === allowed || command.startsWith(`${allowed} `) ||
    (allowed === '--version' && args[0] === allowed));
  if (!known) throw new Error(`Refusing gcloud ${words.slice(0, 3).join(' ')}: not a command this kit runs.`);
  const group = words.find((word) => BILLABLE_GROUPS.has(word));
  const verb = words[words.indexOf(group) + 1];
  if (group && words[0] === 'compute' && !READ_VERBS.has(verb)) {
    throw new Error(`Refusing gcloud compute ${group} ${verb}: it could bill.`);
  }
  const flag = BILLABLE_FLAGS.find((name) => flags.has(name));
  if (flag) throw new Error(`Refusing ${flag}: it adds something Google bills.`);
  if (command.startsWith('compute instances create')) assertFreeInstance(args);
}

/** Lists every way a described instance and its disks would be billed. */
export function instanceProblems(instance, disks) {
  const problems = [];
  const machine = String(instance.machineType ?? '').split('/').pop();
  const zone = String(instance.zone ?? '').split('/').pop();
  if (machine !== FREE_MACHINE_TYPE) problems.push(`machine type ${machine} is not ${FREE_MACHINE_TYPE}`);
  if (!freeZone(zone)) problems.push(`zone ${zone} is not in ${FREE_REGIONS.join(', ')}`);
  const scheduling = instance.scheduling ?? {};
  if (scheduling.preemptible || (scheduling.provisioningModel ?? STANDARD_PROVISIONING) !== STANDARD_PROVISIONING) {
    problems.push('Spot or preemptible machines are not in the free tier');
  }
  if ((instance.guestAccelerators ?? []).length) problems.push('a GPU is attached');
  for (const nic of instance.networkInterfaces ?? []) {
    if ((nic.accessConfigs ?? []).length) problems.push('an external IPv4 address is attached (billed hourly)');
  }
  if ((instance.disks ?? []).length !== 1) problems.push(`${(instance.disks ?? []).length} disks attached; only one boot disk is free`);
  let total = 0;
  for (const disk of disks) {
    const type = String(disk.type ?? '').split('/').pop();
    if (type !== FREE_DISK_TYPE) problems.push(`disk ${disk.name} is ${type}, not ${FREE_DISK_TYPE}`);
    total += SIZE_GB.test(String(disk.sizeGb)) ? Number(disk.sizeGb) : Infinity;
  }
  if (total > FREE_DISK_GB) problems.push(`${total} GB of disk is over the free ${FREE_DISK_GB} GB`);
  return problems;
}

/** Lists billable resources anywhere in the project, besides the kit's own machine and boot disk. */
export function auditProblems(inventory, instanceName) {
  const named = (items) => items.map((item) => item.name).join(', ');
  const problems = [];
  const others = (inventory.instances ?? []).filter((item) => item.name !== instanceName);
  const disks = (inventory.disks ?? []).filter((item) => item.name !== instanceName);
  const groups = [
    ['other instances', others], ['other disks', disks], ['snapshots', inventory.snapshots ?? []],
    ['static or reserved IP addresses', inventory.addresses ?? []],
    ['forwarding rules (load balancers)', inventory.forwardingRules ?? []],
    ['Cloud Routers (NAT)', inventory.routers ?? []], ['custom images', inventory.images ?? []],
  ];
  for (const [what, items] of groups) {
    if (items.length) problems.push(`${what}: ${named(items)}`);
  }
  return problems;
}

/** The gcloud commands that create the network, the machine's read-only account and the machine. */
export function machinePlan({ project, zone, instance }) {
  const region = zone.slice(0, zone.lastIndexOf('-'));
  const account = `${VM_ACCOUNT}@${project}.iam.gserviceaccount.com`;
  return [
    {
      what: 'custom VPC network (free)',
      check: ['compute', 'networks', 'describe', NETWORK],
      create: ['compute', 'networks', 'create', NETWORK, '--subnet-mode=custom'],
    },
    {
      what: 'dual-stack subnet: internal IPv4, external IPv6 (IPv6 on a VM is not billed)',
      check: ['compute', 'networks', 'subnets', 'describe', SUBNET, `--region=${region}`],
      create: ['compute', 'networks', 'subnets', 'create', SUBNET, `--network=${NETWORK}`, `--region=${region}`,
        `--range=${SUBNET_RANGE}`, '--stack-type=IPV4_IPV6', '--ipv6-access-type=EXTERNAL',
        '--enable-private-ip-google-access'],
    },
    {
      what: 'SSH only through Identity-Aware Proxy',
      check: ['compute', 'firewall-rules', 'describe', FIREWALL_RULE],
      create: ['compute', 'firewall-rules', 'create', FIREWALL_RULE, `--network=${NETWORK}`, '--direction=INGRESS',
        '--action=allow', '--rules=tcp:22', `--source-ranges=${IAP_RANGE}`],
    },
    {
      what: 'service account the guard uses to read its own disk type',
      check: ['iam', 'service-accounts', 'describe', account],
      create: ['iam', 'service-accounts', 'create', VM_ACCOUNT, '--display-name=Money 24 machine, read-only'],
      grants: [['projects', 'add-iam-policy-binding', project, `--member=serviceAccount:${account}`,
        '--role=roles/compute.viewer', '--condition=None']],
    },
    {
      what: `${FREE_MACHINE_TYPE} with a ${FREE_DISK_GB} GB ${FREE_DISK_TYPE} boot disk and no external IPv4`,
      check: ['compute', 'instances', 'describe', instance, `--zone=${zone}`],
      create: ['compute', 'instances', 'create', instance, `--zone=${zone}`, `--machine-type=${FREE_MACHINE_TYPE}`,
        `--network-interface=subnet=${SUBNET},stack-type=IPV4_IPV6,ipv6-network-tier=PREMIUM,no-address`,
        `--image-family=${IMAGE_FAMILY}`, `--image-project=${IMAGE_PROJECT}`,
        `--boot-disk-type=${FREE_DISK_TYPE}`, `--boot-disk-size=${FREE_DISK_GB}GB`, '--boot-disk-auto-delete',
        `--provisioning-model=${STANDARD_PROVISIONING}`, '--maintenance-policy=MIGRATE',
        `--service-account=${account}`, '--scopes=compute-ro',
        '--shielded-secure-boot', '--shielded-vtpm', '--shielded-integrity-monitoring',
        '--metadata=enable-oslogin=TRUE'],
    },
  ];
}

/** Read-only listings for the project audit, keyed like auditProblems expects. */
export const AUDIT_LISTS = {
  instances: ['compute', 'instances', 'list'],
  disks: ['compute', 'disks', 'list'],
  snapshots: ['compute', 'snapshots', 'list'],
  addresses: ['compute', 'addresses', 'list'],
  forwardingRules: ['compute', 'forwarding-rules', 'list'],
  routers: ['compute', 'routers', 'list'],
  images: ['compute', 'images', 'list', '--no-standard-images'],
};
