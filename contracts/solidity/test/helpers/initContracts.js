import { duration } from './increaseTime';
import { bls } from './data';

async function initContracts(KeepToken, TokenStaking, KeepRandomBeaconService,
  KeepRandomBeaconServiceImplV1, KeepRandomBeaconOperator) {

  let token, stakingContract,
    serviceContractImplV1, serviceContractProxy, serviceContract,
    operatorContract;

  let minimumGasPrice = web3.utils.toBN(20).mul(web3.utils.toBN(10**9)), // (20 Gwei) TODO: Use historical average of recently served requests?
    minimumCallbackAllowance = web3.utils.toBN(200000), // Minimum gas required for relay request callback.
    profitMargin = 1, // Signing group reward per each member in % of the entry fee.
    createGroupFee = 10, // Fraction in % of the estimated cost of group creation that is included in relay request payment.
    withdrawalDelay = 1;

  // Initialize Keep token contract
  token = await KeepToken.new();

  // Initialize staking contract
  stakingContract = await TokenStaking.new(token.address, duration.days(30));

  // Initialize Keep Random Beacon service contract
  serviceContractImplV1 = await KeepRandomBeaconServiceImplV1.new();
  serviceContractProxy = await KeepRandomBeaconService.new(serviceContractImplV1.address);
  serviceContract = await KeepRandomBeaconServiceImplV1.at(serviceContractProxy.address)

  // Initialize Keep Random Beacon operator contract
  operatorContract = await KeepRandomBeaconOperator.new();
  await operatorContract.initialize(serviceContract.address);

  await serviceContract.initialize(minimumGasPrice, minimumCallbackAllowance, profitMargin, createGroupFee, withdrawalDelay, operatorContract.address);

  // TODO: replace with a secure authorization protocol (addressed in RFC 4).
  await operatorContract.authorizeStakingContract(stakingContract.address);

  // Add initial funds to the fee pool to trigger group creation without waiting for fee accumulation
  let createGroupGasEstimateCost = await operatorContract.createGroupGasEstimate();
  await serviceContract.fundCreateGroupFeePool({value: createGroupGasEstimateCost.mul(minimumGasPrice)});

  await operatorContract.genesis();

  return {
    token: token,
    stakingContract: stakingContract,
    serviceContract: serviceContract,
    operatorContract: operatorContract
  };
};

module.exports.initContracts = initContracts;
