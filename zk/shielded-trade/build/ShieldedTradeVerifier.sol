// SPDX-License-Identifier: GPL-3.0
/*
    Copyright 2021 0KIMS association.

    This file is generated with [snarkJS](https://github.com/iden3/snarkjs).

    snarkJS is a free software: you can redistribute it and/or modify it
    under the terms of the GNU General Public License as published by
    the Free Software Foundation, either version 3 of the License, or
    (at your option) any later version.

    snarkJS is distributed in the hope that it will be useful, but WITHOUT
    ANY WARRANTY; without even the implied warranty of MERCHANTABILITY
    or FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public
    License for more details.

    You should have received a copy of the GNU General Public License
    along with snarkJS. If not, see <https://www.gnu.org/licenses/>.
*/

pragma solidity >=0.7.0 <0.9.0;

contract Groth16Verifier {
    // Scalar field size
    uint256 constant r    = 21888242871839275222246405745257275088548364400416034343698204186575808495617;
    // Base field size
    uint256 constant q   = 21888242871839275222246405745257275088696311157297823662689037894645226208583;

    // Verification Key data
    uint256 constant alphax  = 8529157678364175194033650431756900059553096407085751569283373154480350795604;
    uint256 constant alphay  = 1678723089954244208994087761889077740147932277573337543881570106735839381503;
    uint256 constant betax1  = 8800965350408083627570526461221505710856518919247010814738139202141698619106;
    uint256 constant betax2  = 16068902389902771628263680779345219249700429404547081618138152229466055267043;
    uint256 constant betay1  = 5232234446480969410578804759205271344896038121691448061455126741588293799547;
    uint256 constant betay2  = 8999014425973856384976772980020625285729737705185528761175826251850986674484;
    uint256 constant gammax1 = 11559732032986387107991004021392285783925812861821192530917403151452391805634;
    uint256 constant gammax2 = 10857046999023057135944570762232829481370756359578518086990519993285655852781;
    uint256 constant gammay1 = 4082367875863433681332203403145435568316851327593401208105741076214120093531;
    uint256 constant gammay2 = 8495653923123431417604973247489272438418190587263600148770280649306958101930;
    uint256 constant deltax1 = 10268663769269079307794136748055745289678822082280629721000279362065710128184;
    uint256 constant deltax2 = 11939023800361436873731392170410080061941232298897695100678680900982605757174;
    uint256 constant deltay1 = 19198423408641893348612413476151480936809817136414833162279964741036927751675;
    uint256 constant deltay2 = 9217300169766018803777658271027226006640388985434901478658900774837818224910;

    
    uint256 constant IC0x = 21281608962547793541820432627601960729093042563371979855460788855815978074732;
    uint256 constant IC0y = 20672042480596861115854926014085021983365177405115824775422940438647266575949;
    
    uint256 constant IC1x = 17219219629228445195738717946764534663474494787811707515203568046091877240982;
    uint256 constant IC1y = 18652104984199441118136818913894953741437361489797713506733585060671895334503;
    
    uint256 constant IC2x = 15899530185256303345455951110224211206280061252585001527826955492739182227931;
    uint256 constant IC2y = 16706803385755404363899234641644440756940092073979171652956644532186097835296;
    
    uint256 constant IC3x = 10256512651575332234376527362886974936980570359164205237934790462891981227462;
    uint256 constant IC3y = 6114209505724933135875365428339816512658407084352057009375914188913759492941;
    
    uint256 constant IC4x = 6589683457995155956890560874937898309700623107020375498295336706037489001911;
    uint256 constant IC4y = 14831061057458888556021370251303746467761058100036828190585516989514862750257;
    
    uint256 constant IC5x = 10739579988882151299842436645170043152516500353089563452695458838507022319678;
    uint256 constant IC5y = 14037803157854014299826373013197913562713858847768910445962929406014422866680;
    
    uint256 constant IC6x = 19347111087797182421465564868305418378230091866272624472132640855947474454996;
    uint256 constant IC6y = 20720719816700952432183434164252881869965340204210820749443977334074074627110;
    
    uint256 constant IC7x = 8796713321229619476475782058549540247181288655269316296051056770595241115782;
    uint256 constant IC7y = 6091324354502250669369131900120117700604423033294233073917236253463427077289;
    
    uint256 constant IC8x = 13399307335010854961644842167190205287392892996301629391912442277647270019236;
    uint256 constant IC8y = 18717417566719949710984823579770037675362399829785526997759688785609051548009;
    
    uint256 constant IC9x = 10073783867735735924388612582197929083636751868482730694813156873033312757988;
    uint256 constant IC9y = 6643838699557951100014686349464457616711874511611559532566233026798367970792;
    
 
    // Memory data
    uint16 constant pVk = 0;
    uint16 constant pPairing = 128;

    uint16 constant pLastMem = 896;

    function verifyProof(uint[2] calldata _pA, uint[2][2] calldata _pB, uint[2] calldata _pC, uint[9] calldata _pubSignals) public view returns (bool) {
        assembly {
            function checkField(v) {
                if iszero(lt(v, r)) {
                    mstore(0, 0)
                    return(0, 0x20)
                }
            }
            
            // G1 function to multiply a G1 value(x,y) to value in an address
            function g1_mulAccC(pR, x, y, s) {
                let success
                let mIn := mload(0x40)
                mstore(mIn, x)
                mstore(add(mIn, 32), y)
                mstore(add(mIn, 64), s)

                success := staticcall(sub(gas(), 2000), 7, mIn, 96, mIn, 64)

                if iszero(success) {
                    mstore(0, 0)
                    return(0, 0x20)
                }

                mstore(add(mIn, 64), mload(pR))
                mstore(add(mIn, 96), mload(add(pR, 32)))

                success := staticcall(sub(gas(), 2000), 6, mIn, 128, pR, 64)

                if iszero(success) {
                    mstore(0, 0)
                    return(0, 0x20)
                }
            }

            function checkPairing(pA, pB, pC, pubSignals, pMem) -> isOk {
                let _pPairing := add(pMem, pPairing)
                let _pVk := add(pMem, pVk)

                mstore(_pVk, IC0x)
                mstore(add(_pVk, 32), IC0y)

                // Compute the linear combination vk_x
                
                g1_mulAccC(_pVk, IC1x, IC1y, calldataload(add(pubSignals, 0)))
                
                g1_mulAccC(_pVk, IC2x, IC2y, calldataload(add(pubSignals, 32)))
                
                g1_mulAccC(_pVk, IC3x, IC3y, calldataload(add(pubSignals, 64)))
                
                g1_mulAccC(_pVk, IC4x, IC4y, calldataload(add(pubSignals, 96)))
                
                g1_mulAccC(_pVk, IC5x, IC5y, calldataload(add(pubSignals, 128)))
                
                g1_mulAccC(_pVk, IC6x, IC6y, calldataload(add(pubSignals, 160)))
                
                g1_mulAccC(_pVk, IC7x, IC7y, calldataload(add(pubSignals, 192)))
                
                g1_mulAccC(_pVk, IC8x, IC8y, calldataload(add(pubSignals, 224)))
                
                g1_mulAccC(_pVk, IC9x, IC9y, calldataload(add(pubSignals, 256)))
                

                // -A
                mstore(_pPairing, calldataload(pA))
                mstore(add(_pPairing, 32), mod(sub(q, calldataload(add(pA, 32))), q))

                // B
                mstore(add(_pPairing, 64), calldataload(pB))
                mstore(add(_pPairing, 96), calldataload(add(pB, 32)))
                mstore(add(_pPairing, 128), calldataload(add(pB, 64)))
                mstore(add(_pPairing, 160), calldataload(add(pB, 96)))

                // alpha1
                mstore(add(_pPairing, 192), alphax)
                mstore(add(_pPairing, 224), alphay)

                // beta2
                mstore(add(_pPairing, 256), betax1)
                mstore(add(_pPairing, 288), betax2)
                mstore(add(_pPairing, 320), betay1)
                mstore(add(_pPairing, 352), betay2)

                // vk_x
                mstore(add(_pPairing, 384), mload(add(pMem, pVk)))
                mstore(add(_pPairing, 416), mload(add(pMem, add(pVk, 32))))


                // gamma2
                mstore(add(_pPairing, 448), gammax1)
                mstore(add(_pPairing, 480), gammax2)
                mstore(add(_pPairing, 512), gammay1)
                mstore(add(_pPairing, 544), gammay2)

                // C
                mstore(add(_pPairing, 576), calldataload(pC))
                mstore(add(_pPairing, 608), calldataload(add(pC, 32)))

                // delta2
                mstore(add(_pPairing, 640), deltax1)
                mstore(add(_pPairing, 672), deltax2)
                mstore(add(_pPairing, 704), deltay1)
                mstore(add(_pPairing, 736), deltay2)


                let success := staticcall(sub(gas(), 2000), 8, _pPairing, 768, _pPairing, 0x20)

                isOk := and(success, mload(_pPairing))
            }

            let pMem := mload(0x40)
            mstore(0x40, add(pMem, pLastMem))

            // Validate that all evaluations ∈ F
            
            checkField(calldataload(add(_pubSignals, 0)))
            
            checkField(calldataload(add(_pubSignals, 32)))
            
            checkField(calldataload(add(_pubSignals, 64)))
            
            checkField(calldataload(add(_pubSignals, 96)))
            
            checkField(calldataload(add(_pubSignals, 128)))
            
            checkField(calldataload(add(_pubSignals, 160)))
            
            checkField(calldataload(add(_pubSignals, 192)))
            
            checkField(calldataload(add(_pubSignals, 224)))
            
            checkField(calldataload(add(_pubSignals, 256)))
            

            // Validate all evaluations
            let isValid := checkPairing(_pA, _pB, _pC, _pubSignals, pMem)

            mstore(0, isValid)
             return(0, 0x20)
         }
     }
 }
