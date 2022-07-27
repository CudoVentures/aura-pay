Initial readme of the tokenised infrastructure rewarder
- Written in golang
- It should connect to the CUDOS network and fetch all NFTS that have expireOn≤currentDateTime and hashRateId(or minerId).
- For every NFT that was fetched it should connect to the bitcoin mining pool  via the pool’s API and get the reward for each hashRate that was fetched from an NFT.
- After that it should connect to a bitcoin node and transfer the reward from the address that the miner distributed their rewards(bitcoin) to the address(bitcoin) of the NFT owner