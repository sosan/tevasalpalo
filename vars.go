package main

// Opcionalmente, si los links tienen más información (como calidad, idioma, etc.)
type AcestreamLink struct {
	Hash     string `json:"hash"`
	Quality  string `json:"quality,omitempty"`  // Ej: "HD", "SD"
	Language string `json:"language,omitempty"` // Ej: "ES", "EN"
}

// Mapeo de broadcasters a su información detallada (incluyendo múltiples links)
var broadcasterToAcestream = map[string]BroadcasterInfo{
	"DAZN 1": {
		Logo:  "dazn1.png",
		Links: []string{
			// "6538b79ce0da657d8455d1da6a5f342398899a0e",
			// "50a8a13c474848d1efbd5586efdb5b6cdd173fa9",
			// "4141892f5df7d6474bf0279895ce02b7336c9928",
			// "36394be1db2e20b5997d987c32fd38c7f0f194b7",
		},
		Name:             "DAZN",
		ShowListChannels: true,
	},
	"DAZN 2": {
		Logo:  "dazn2.png",
		Links: []string{
			"8b081c8afbd9beafc8c5fbf0115eb36eadb07a35",
			// "43004e955731cd3afcc34d24e5178d4b427bff37",
			// "b0eabe0fdd02fdd165896236765a9b753a2ff516",
		},
		Name:             "DAZN 2",
		ShowListChannels: true,
	},
	"DAZN 3": {
		Logo:             "dazn3.png",
		Links:            []string{
			"d1d9ec2361a6ac8edc0b2841866383768cc28df9",
		},
		Name:             "DAZN 3",
		ShowListChannels: true,
	},
	"DAZN 4": {
		Logo:  "dazn4.png",
		Links: []string{
			"eb884f77ce8815cf1028c4d85e8b092c27ea1693",
			// "4e401fdceebffdf1f09aef954844d09f6c62f460",
			// "eb884f77ce8815cf1028c4d85e8b092c27ea1693",
			// "6a11eb510edc5b3581c5c97883c44563eb894b1b",
			// "7e90956539f4e1318a63f3960e4f75c7c7c5a3b8",
			// "c21a2524a8de3e1e5b126f2677a3e993d9aa07c4",
		},
		Name:             "DAZN 4",
		ShowListChannels: true,
	},
	"DAZN LALIGA": {
		Logo:  "daznlaliga.png",
		Links: []string{
			"1bb5bf76fb2018d6db9aaa29b1467ecdfabe2884",
			"74defb8f4ed3a917fd07c21b34f43c17107ec618",
			// "0e50439e68aa2435b38f0563bb2f2e98f32ff4b1",
			// "1bb5bf76fb2018d6db9aaa29b1467ecdfabe2884",
			// "f8d5e39a49b9da0215bbd3d9efb8fb3d06b76892",
		},
		Name:             "DAZN LALIGA 1",
		ShowListChannels: true,
	},
	"DAZN LALIGA 2": {
		Logo:  "daznlaliga.png",
		Links: []string{
			"5091ea94b75ba4b50b078b4102a3d0e158ef59c3",
			"c976c7b37964322752db562b4ad65515509c8d36",
		},
		Name:             "DAZN LALIGA 2",
		ShowListChannels: true,
	},

	"DAZN 1 Bar": {
		Logo: "dazn1bar.png",
		Links: []string{
			"688e785893b50acc1d00cb37f15bfc42e72f5ae3",
		},
		Name:             "DAZN BAR",
		ShowListChannels: true,
	},
	"DAZN 2 Bar": {
		Logo: "dazn2bar.png",
		Links: []string{
			"2f9211669499f413dab1a490198afab2a9b4b57c",
		},
		Name:             "DAZN 2 BAR",
		ShowListChannels: true,
	},
	"DS SPORT": {
		Logo: "dsport.webp",
		Links: []string{
			"8bdeb6055da0be3bd1e1977dbf3640408f7d0267",
			"http://190.117.20.37:8000/play/a08d/index.m3u8",   //
			"http://179.63.6.17:9000/play/a08e",                //
			"http://181.78.106.127:9000/play/ca026/index.m3u8", //
		},
		Name:             "DSPORT",
		ShowListChannels: true,
	},
	"ESPN MX": {
		Logo: "espn.webp",
		Links: []string{
			"http://181.78.106.127:9000/play/ca033/index.m3u8", //"",
		},
		Name:             "ESPN MX",
		ShowListChannels: true,
	},
	// "ESPN MX 2": {
	// 	Logo: "espn.webp",
	// 	Links: []string{
	// 		"http://181.78.106.127:9000/play/ca033/index.m3u8", //"",
	// 	},
	// 	Name: "ESPN MX 2",
	// 	ShowListChannels: true,
	// },
	"ESPN 2": {
		Logo: "espn.webp",
		Links: []string{
			"http://45.71.254.1:8000/play/a0e7/index.m3u8",
			"http://181.78.106.127:9000/play/ca034/index.m3u8",
			// "http://181.205.130.194:4000/play/a07i",
		},
		Name:             "ESPN 2",
		ShowListChannels: true,
	},
	"ESPN": {
		Logo: "espn.webp",
		Links: []string{
			"http://181.188.216.5:18000/play/a0dk/index.m3u8",
			"http://181.78.106.127:9000/play/ca033/index.m3u8",

			// "http://45.71.254.1:8000/play/a0e6/index.m3u8",
			// "http://200.60.124.19:29000/play/a015",
			// "http://181.78.106.127:9000/play/ca034/index.m3u8",
			// "http://181.205.130.194:4000/play/a07i",            //

		},
		Name:             "ESPN 1",
		ShowListChannels: true,
	},

	"ESPN 3": {
		Logo: "espn.webp",
		Links: []string{
			"http://45.71.254.1:8000/play/a0e8/index.m3u8",
			"http://181.78.106.127:9000/play/ca035/index.m3u8", //
		},
		Name:             "ESPN 3",
		ShowListChannels: true,
	},
	"ESPN 4": {
		Logo: "espn.webp",
		Links: []string{
			"http://45.71.254.1:8000/play/a0c5/index.m3u8",
		},
		Name:             "ESPN 4",
		ShowListChannels: true,
	},

	"ESPN 5": {
		Logo: "espn.webp",
		Links: []string{
			"http://179.51.136.19:8000/play/a1a6/index.m3u8",
			// "http://38.41.8.1:8000/play/a07b", //
		},
		Name:             "ESPN 5",
		ShowListChannels: true,
	},

	"ESPN 6": {
		Logo: "espn.webp",
		Links: []string{
			"http://45.71.254.1:8000/play/a0e9/index.m3u8",
			// "http://181.205.130.194:4000/play/a07t", //
			// "http://38.44.109.41:8003/play/a00u",    //
			// "http://38.41.8.1:8000/play/a082",       //
		},
		Name:             "ESPN 6",
		ShowListChannels: true,
	},

	"ESPN 7 MX": {
		Logo: "espn.webp",
		Links: []string{
			"http://181.205.130.194:4000/play/a07s", //

		},
		Name:             "ESPN 7 MX",
		ShowListChannels: true,
	},
	"ESPN DEPORTES": {
		Logo: "espndeportes.png",
		Links: []string{
			"https://tvpass.org/live/espn-deportes/sd",
			"4b048bcfaed7daec454e88f0e29b56756300447d",
		},
		Name:             "ESPN DEPORTES",
		ShowListChannels: true,
	},
	"ESPN PREMIUM": {
		Logo: "espnpremium.webp",
		Links: []string{
			"http://190.102.246.93:9005/play/a00x", //
			"http://190.102.246.93:9005/play/a029", //
		},
		Name:             "ESPN PREMIUM",
		ShowListChannels: true,
	},
	"TNT": {
		Logo: "tntsports.png",
		Links: []string{
			"efc60cfe5e3a349baa02bcc49f6647c21a9c3c5b",
		},
		Name:             "TNT SPORTS 1",
		ShowListChannels: false,
	},
	"TNT SPORTS": {
		Logo: "tntsports.png",
		Links: []string{
			"efc60cfe5e3a349baa02bcc49f6647c21a9c3c5b",
		},
		Name:             "TNT SPORTS 1",
		ShowListChannels: true,
	},
	"TNT SPORTS 1": {
		Logo: "tntsports.png",
		Links: []string{
			"efc60cfe5e3a349baa02bcc49f6647c21a9c3c5b",
		},
		Name:             "TNT SPORTS 1",
		ShowListChannels: false,
	},
	"TNT SPORTS 2": {
		Logo: "tntsports.png",
		Links: []string{
			"d63d8a57cf471394bfa9f619bbd68b01ae27a801",
			"",
		},
		Name:             "TNT SPORTS 2",
		ShowListChannels: true,
	},
	"TNT SPORTS 3": {
		Logo: "tntsports.png",
		Links: []string{
			"5f966c123759de46dff29c379266b7a403452033",
		},
		Name:             "TNT SPORTS 3",
		ShowListChannels: true,
	},
	"TNT SPORTS 4": {
		Logo: "tntsports.png",
		Links: []string{
			"fc5089d8e1519872fdf951779ccbca913acc9bce",
		},
		Name:             "TNT SPORTS 4",
		ShowListChannels: true,
	},
	"DAZN F1": {
		Logo:  "daznf1.png",
		Links: []string{
			"1a63d886860e2b770590dcd64a4dd472eabb841d",
			"38e9ae1ee0c96d7c6187c9c4cc60ffccb565bdf7",
			"3f5b7e2f883fe4b4b973e198d147a513d5c7c32a",
			// "38e9ae1ee0c96d7c6187c9c4cc60ffccb565bdf7",
			// "3f5b7e2f883fe4b4b973e198d147a513d5c7c32a",
			// "ba6e1bdc8e03da60ff572557645eb04370af0cd0",
			// "8c1155cdfae76eb582290c04904c98da066657c9",
			// "b08e158ea3f5c72084f5ff8e3c30ca2e4d1ff6d1",
			// "bcf9dc38f92e90a71b87bd54b3bac91b76d09a69",
			// "fd53cfa7055fe458d4f5c0ff59a06cd43723be55",
			// "ed6188dcbb491efeb2092c1a6199226c27f61727",
			// "6422e8bc34282871634c81947be093c04ad1bb29",
		},
		Name:             "DAZN F1",
		ShowListChannels: true,
	},
	"DAZN LALIGA TV": {
		Logo:  "daznlaliga.png",
		Links: []string{
			"1bb5bf76fb2018d6db9aaa29b1467ecdfabe2884",
			"74defb8f4ed3a917fd07c21b34f43c17107ec618",
			// "0e50439e68aa2435b38f0563bb2f2e98f32ff4b1",
			// "1bb5bf76fb2018d6db9aaa29b1467ecdfabe2884",
			// "f8d5e39a49b9da0215bbd3d9efb8fb3d06b76892",
			// "520950d296c952e1864a08e15af9f89f1ab514ec",
		},
		Name:             "DAZN LA LIGA 1",
		ShowListChannels: false,
	},
	"M+ LALIGA": {
		Logo:  "mlaliga.png",
		Links: []string{
			"d2ddf9802ccfdc456f872eea4d24596880a638a0",
			"c9321006921967d6258df6945f1d598a5c0cbf1e",
			"107c3ce5a5d2527c9f06e4eab87477201791f231",
			// "107c3ce5a5d2527c9f06e4eab87477201791f231",
			// "d2ddf9802ccfdc456f872eea4d24596880a638a0",
			// "14b6cd8769cd485f2cffdca64be9698d9bfeac58",
			// "07f2b92762cfff99bba785c2f5260c7934ca6034",
			// "4b528d10eaad747ddf52251206177573ee3e9f74",
			// "d3de78aebe544611a2347f54d5796bd87f16c92d",
		},
		Name:             "M+ LALIGA",
		ShowListChannels: true,
	},
	"M+ LALIGA TV": {
		Logo:  "mlaligatv.png",
		Links: []string{
			"d2ddf9802ccfdc456f872eea4d24596880a638a0",
			"c9321006921967d6258df6945f1d598a5c0cbf1e",
			"107c3ce5a5d2527c9f06e4eab87477201791f231",
			// "14b6cd8769cd485f2cffdca64be9698d9bfeac58",
			// "07f2b92762cfff99bba785c2f5260c7934ca6034",
			// "4b528d10eaad747ddf52251206177573ee3e9f74",
			// "d3de78aebe544611a2347f54d5796bd87f16c92d",
		},
		Name:             "M+ LALIGA",
		ShowListChannels: false,
	},
	"M+ LALIGA 2": {
		Logo:  "mlaliga2.png",
		Links: []string{
			"911ad127726234b97658498a8b790fdd7516541d",
			// "51b363b1c4d42724e05240ad068ad219df8042ec",
			// "911ad127726234b97658498a8b790fdd7516541d",
			"ad42faa399df66dcd62a1cbc9d1c99ed4512d3b8",
		},
		Name:             "M+ LALIGA 2",
		ShowListChannels: true,
	},
	"M+ LALIGA 2 TV": {
		Logo:  "mlaliga2.png",
		Links: []string{
			"911ad127726234b97658498a8b790fdd7516541d",
			"51b363b1c4d42724e05240ad068ad219df8042ec",
			// "911ad127726234b97658498a8b790fdd7516541d",
			// "51b363b1c4d42724e05240ad068ad219df8042ec",
			// "ad42faa399df66dcd62a1cbc9d1c99ed4512d3b8",
		},
		Name:             "M+ LALIGA 2",
		ShowListChannels: false,
	},
	"M+ LALIGA 3": {
		Logo:  "mlaliga3.png",
		Links: []string{
			"7ad14386deef2f45ffe17d30a631dbf79b6a1a87",
			// "382b14499e3d76e557d449d2e5bbc4e4bd63bd39",
		},
		Name:             "M+ LALIGA 3",
		ShowListChannels: true,
	},
	"M+ LALIGA 3 TV": {
		Logo:  "mlaliga3.png",
		Links: []string{
			"7ad14386deef2f45ffe17d30a631dbf79b6a1a87",
			// "382b14499e3d76e557d449d2e5bbc4e4bd63bd39",
		},
		Name:             "M+ LALIGA 3",
		ShowListChannels: false,
	},
	"M+ LIGA DE CAMPEONES": {
		Logo:  "mligacampeones.png",
		Links: []string{
			"c16b4fab1f724c94cad92081cbb7fc7c6fe8a2cc",
			// "0f7842f8b6c26571e5a974407b61623e56e6a052",
			// "f3eea003e23f94dc2d527306de9dd386e3ebf4ba",
			// "680187938f9305cce3ae240298f10e8695bf77c2",
			// "e572a5178ff72eed7d1d751a18b4b3419699f370",
			// "c16b4fab1f724c94cad92081cbb7fc7c6fe8a2cc",
			// "afbf2a479c5a5072698139f0f556ef3e77a99bd0",
			// "dfa66881b9613a77bd5479f6eedc5542504c3ef7",
		},
		Name:             "M+ LIGA DE CAMPEONES 1",
		ShowListChannels: true,
	},
	"M+ LIGA DE CAMPEONES 2": {
		Logo: "mligacampeones2.png",
		Links: []string{
			// "38f7b2044e549df2039ff26cefa6f9a60c854d5e",
			"c6a3673f6a37b1bd3cf31fdd6404dd33d48cfccb",
			"4fc6d0331830ad8743abab2fe2473b63bdfbc49f",
			// "e7d8cae7f693fe46e89bbf74820caac9ffb32a30",
			// "33c009a025508cb2186b9ce36279640bb2507bdf",
			// "74ab4e4ec7e2da001f473ca40893b7307b8029c5",
			// "4fc6d0331830ad8743abab2fe2473b63bdfbc49f",
		},
		Name:             "M+ LIGA DE CAMPEONES 2",
		ShowListChannels: true,
	},
	"M+ LIGA DE CAMPEONES 3": {
		Logo:  "mligacampeones3.png",
		Links: []string{
			"17b8bc4bf8e29547b0071c742e3d7da3bcbc484d",
			// "2b5129adc57d43790634d796fe3468b9fd061259",
			// "17b8bc4bf8e29547b0071c742e3d7da3bcbc484d",
			// "4416843c96b7f7a1bc55c476091a60fff0922bc7",
		},
		Name:             "M+ LIGA DE CAMPEONES 3",
		ShowListChannels: true,
	},
	"M+ LIGA DE CAMPEONES 4": {
		Logo: "mligacampeones4.png",
		Links: []string{
			"77998f8161373611ff6b348e7eda5b4e97d3ec29",
		},
		Name:             "M+ LIGA DE CAMPEONES 4",
		ShowListChannels: true,
	},
	"M+ LIGA DE CAMPEONES 5": {
		Logo: "mligacampeones5.png",
		Links: []string{
			"5620c0ce3dcbf14a6375cb2c2d681207f45eb97d",
		},
		Name:             "M+ LIGA DE CAMPEONES 5",
		ShowListChannels: true,
	},
	"M+ LIGA DE CAMPEONES 6": {
		Logo: "mligacampeones6.png",
		Links: []string{
			
		},
		Name:             "M+ LIGA DE CAMPEONES 6",
		ShowListChannels: true,
	},
	"M+ LIGA DE CAMPEONES 7": {
		Logo: "mligacampeones7.png",
		Links: []string{
			
		},
		Name:             "M+ LIGA DE CAMPEONES 7",
		ShowListChannels: true,
	},
	"M+ LIGA DE CAMPEONES 8": {
		Logo: "mligacampeones8.png",
		Links: []string{
			
		},
		Name:             "M+ LIGA DE CAMPEONES 8",
		ShowListChannels: true,
	},
	"M+ DEPORTES": {
		Logo:  "mdeportes.png",
		Links: []string{
			// "5d3f582738467aaf213e408601aca5cc13fa9359",
			// "3692ea4cdda97eb0062ed5d656ebd61f149ebd0b",
			// "751b9cb03d188ce70e6aac22c1bfb16a5d0b2237",
			// "ef9dcc4eaac36a0f608b52a31f8ab237859e071a",
			// "ebca4a63ce3bfda7b272964a1acc5227218184a4",
			// "2f3cfd199a49819cbd129689a840dc3d23ab93aa",
		},
		Name:             "M+ DEPORTES 1",
		ShowListChannels: true,
	},
	"M+ DEPORTES 2": {
		Logo:  "mdeportes2.png",
		Links: []string{
			// "73d38feaa770d788848ec098470e32670fe55a61",
			// "438991226c3bc6a06e7bfda9bea9f769957dc366",
			// "f0ee7a2b43c1df5ea9e4fac5bf876d5bef4372b0",
			// "edd06f11e1cef292a1d795e15207ef2f580e298c",
			// "bfa01c11c5c6b7a616a516de4f2c769a89d26b25",
		},
		Name:             "M+ DEPORTES 2",
		ShowListChannels: true,
	},
	"M+ DEPORTES 3": {
		Logo:  "mdeportes3.png",
		Links: []string{
			// "29d786d72d4b53dbc333af3a50f8fb021aa0296f",
			// "d5271a967767f761c8812c4b6195dd40f20126f7",
			// "753d4b1f7c4ef43238b5cf23b05572b550a44eee",
		},
		Name:             "M+ DEPORTES 3",
		ShowListChannels: true,
	},
	"M+ DEPORTES 4": {
		Logo:  "mdeportes4.png",
		Links: []string{
			// "37825883ed185365e3194a79207347f6c7bd5ba5",
			// "ebf3f251c1e119aefc6a1efc95c9b5d1909249e2",
			// "58a4c880ab18486d914751db32a12760e74b75a4",
		},
		Name:             "M+ DEPORTES 4",
		ShowListChannels: true,
	},
	"M+ DEPORTES 5": {
		Logo:  "mdeportes5.png",
		Links: []string{
			// "6dc4ac4eeae18e9daec433b01db82435cf84c57c",
			// "9b84af74b2fa3690c519199326fc2f181b025cdd",
			// "0b708083541a46dc15216c8003d7bcf39c458b2a",
		},
		Name:             "M+ DEPORTES 5",
		ShowListChannels: true,
	},
	"M+ DEPORTES 6": {
		Logo:  "mdeportes6.png",
		Links: []string{
			// "190a81938c2ddc6fe97758271f8c48f4db31c28a",
		},
		Name:             "M+ DEPORTES 6",
		ShowListChannels: true,
	},

	"M+ VAMOS": {
		Logo: "mvamos.png",
		Links: []string{
			"0e5d8c9724fa9163f49096b70484e315251eb785",
			// "4e99e755aa32c4bc043a4bb1cd1de35f9bd94dde",
			// "1444a976d2cf6e7fdcee833ed36ee5e55632253f",
			// "c7c81acdd1a03ecc418c94c2f28e2adb0556c40b",
			// "3b2a8b41e7097c16b0468b42d7de0320886fa933",
			// "2940120bf034db79a7f5849846ccea0255172eae",
		},
		Name:             "M+ VAMOS",
		ShowListChannels: true,
	},

	// "M+ GOLF": {
	// 	Logo:  "mgolf.png",
	// 	Links: []string{
	// 		// "76a69812c66bfc4899e89df498220588a56e6064",
	// 		// "872608e734992db636eb79426802cd08f4029afb",
	// 	},
	// },
	// "Movistar Golf": {
	// 	Logo:  "mgolf.png",
	// 	Links: []string{
	// 		// "76a69812c66bfc4899e89df498220588a56e6064",
	// 		// "872608e734992db636eb79426802cd08f4029afb",
	// 	},
	// },
	"Primera Federacion": {
		Logo: "primerafederacion.png",
		Links: []string{
			"b40212c43e96e97542ea00f2129212a054853a57",
		},
		Name:             "Primera federacion",
		ShowListChannels: true,
	},
	"Ten TV": {
		Logo: "ten.png",
		Links: []string{
			"19cab799c86251ae8ae5f4b4faace9b78d784abc",
		},
		Name:             "TEN TV",
		ShowListChannels: true,
	},
	"M+": {
		Logo:  "m.png",
		Links: []string{
			// "199190e3f28c1de0be34bccf0d3568dc65209b99",
			// "5866e83279307bf919068ae7a0d250e4e424e464",
			// "5d9a26e0a5f3e5f2ae027bd05635ab9a4fd4b51a",
			// "5e24a1b9187fccb91553f7c7da4b36341386f74a",
			// "1ab443f5b4beb6d586f19e8b25b9f9646cf2ab78",
		},
		Name:             "M+",
		ShowListChannels: false,
	},
	"Movistar Plus+": {
		Logo:  "m.png",
		Links: []string{
			// "199190e3f28c1de0be34bccf0d3568dc65209b99",
			// "5866e83279307bf919068ae7a0d250e4e424e464",
			// "5d9a26e0a5f3e5f2ae027bd05635ab9a4fd4b51a",
			// "5e24a1b9187fccb91553f7c7da4b36341386f74a",
		},
		Name:             "Movistar Plus+ 1",
		ShowListChannels: true,
	},
	"Movistar Plus+ 2": {
		Logo:             "m2.png",
		Links:            []string{
			"e19c1fc5e3ada56c60d45257f7f4ed0d14bf7003",
		},
		Name:             "Movistar Plus+ 2",
		ShowListChannels: true,
	},
	"Movistar Plus": {
		Logo:  "m.png",
		Links: []string{
			// "199190e3f28c1de0be34bccf0d3568dc65209b99",
			// "5866e83279307bf919068ae7a0d250e4e424e464",
			// "5d9a26e0a5f3e5f2ae027bd05635ab9a4fd4b51a",
			// "5e24a1b9187fccb91553f7c7da4b36341386f74a",
		},
		Name:             "Movistar Plus+ 1",
		ShowListChannels: false,
	},
	"LALIGA TV HYPERMOTION": {
		Logo:  "mlaligahyper.png",
		Links: []string{
			// "8ee52f6208e33706171856f99d2ed2dabd317f3a",
			// "70f22be1286ef224b5e4e9451d9a42468152cda4",
			// "f15f997f457e49ad9697e65cf2d78db26ee875b9",
			// "ff38b875b60074d60edb64cf10d09b32370a7135",
			// "778d2f60bb7207addedcca0b9aed98f41529724e",
		},
		Name:             "LALIGA TV HYPERMOTION",
		ShowListChannels: true,
	},
	"LALIGA TV HYPERMOTION 2": {
		Logo: "mlaligahyper.png",
		Links: []string{
			"8a05571c0c8fe53f160fb7d116cdf97243679e86",
		},
		Name:             "LALIGA TV HYPERMOTION 2",
		ShowListChannels: true,
	},
	"LALIGA TV HYPERMOTION 3": {
		Logo: "mlaligahyper.png",
		Links: []string{
			"1ba18731a8e18bb4b3a5dfa5bb7b0f05762849a6",
		},
		Name:             "LALIGA TV HYPERMOTION 3",
		ShowListChannels: true,
	},
	// "LALIGA TV HYPERMOTION": {
	// 	Logo:  "mlaligahyper.png",
	// 	Links: []string{
	// 		// "8ee52f6208e33706171856f99d2ed2dabd317f3a",
	// 		// "70f22be1286ef224b5e4e9451d9a42468152cda4",
	// 		// "f15f997f457e49ad9697e65cf2d78db26ee875b9",
	// 		// "ff38b875b60074d60edb64cf10d09b32370a7135",
	// 		// "778d2f60bb7207addedcca0b9aed98f41529724e",
	// 	},
	// 	Name: "LALIGA TV HYPERMOTION",
	// 	ShowListChannels: true,
	// },

	"GOL": {
		Logo: "gol.png",
		Links: []string{
			"b2d560741c006fc5e4a42412bb52dbd25a6a4a3a",
			// "472d1f3a658a51bcab0ffa9138e1e28a05fba30b",
		},
		Name:             "GOL",
		ShowListChannels: true,
	},
	"GOL TV": {
		Logo: "goltv.png",
		Links: []string{
			"http://181.188.216.5:18000/play/a0mj/index.m3u8",
			// "b2d560741c006fc5e4a42412bb52dbd25a6a4a3a",
			// "472d1f3a658a51bcab0ffa9138e1e28a05fba30b",
		},
		Name:             "GOL TV",
		ShowListChannels: true,
	},
	"GOLTV PLAY": {
		Logo: "golt.png",
		Links: []string{
			"b2d560741c006fc5e4a42412bb52dbd25a6a4a3a",
			// "472d1f3a658a51bcab0ffa9138e1e28a05fba30b",
			// "b2d560741c006fc5e4a42412bb52dbd25a6a4a3a",
		},
		Name:             "GOLTV PLAY",
		ShowListChannels: true,
	},
	"EUROSPORT 1": {
		Logo:  "eurosport1.png",
		Links: []string{
			// "ef2abf419586d9876370be127ad592dbb41b126a",
			// "bae98f69fbf867550b4f73b4eb176dae84d7f909",
			// "714e14e6d6e27660fd25a75b57b0ac5580fe705d",
		},
		Name:             "EUROSPORT 1",
		ShowListChannels: true,
	},
	"EUROSPORT 2": {
		Logo:  "eurosport2.png",
		Links: []string{
			// "37d6f1aabcde81ee6e4873b4db6b7bb8964af8bf",
			// "dc4ccb9e72550bc72be9360aa7d77e337ad11ecc",
			// "0585e09bb8ac9720e4c11934f1b184e309291551",
			// "5c910d614894635153a7d42de98cc2e4a958a53f",
		},
		Name:             "EUROSPORT 2",
		ShowListChannels: true,
	},
	"M+ ELLAS VAMOS": {
		Logo:  "mellasvamos.png",
		Links: []string{
			// "9b84af74b2fa3690c519199326fc2f181b025cdd",
		},
		Name:             "M+ ELLAS VAMOS",
		ShowListChannels: true,
	},
	// "LALIGA TV BAR": {
	// 	Logo:  "laligatvbar.png",
	// 	Links: []string{
	// 		// "608b0faf7d3d25f6fe5dba13d5e4b4142949990e",
	// 	},
	// 	Name: "LALIGA TV BAR 1",
	// 	ShowListChannels: true,
	// },

	"TUDN": {
		Logo: "tudn.png",
		Links: []string{
			"http://181.78.106.127:9000/play/ca036/index.m3u8", //
		},
		Name:             "TUDN 1",
		ShowListChannels: true,
	},

	"FOX SPORTS 1": {
		Logo: "foxsports1.png",
		Links: []string{
			"http://181.78.106.127:9000/play/ca031/index.m3u8", //
			"http://200.115.120.1:8000/play/ca041/index.m3u8",  //
		},
		Name:             "FOX SPORTS 1",
		ShowListChannels: true,
	},
	"FOX SPORTS 2": {
		Logo: "foxsports1.png",
		Links: []string{
			"http://181.78.106.127:9000/play/ca032/index.m3u8", //
		},
		Name:             "FOX SPORTS 2",
		ShowListChannels: true,
	},

	"FOX SPORTS MX": {
		Logo: "foxsports1.png",
		Links: []string{
			"a0464d3642e054f6122e7c309017d1e8d23f4da9", //
		},
		Name:             "FOX SPORTS MX",
		ShowListChannels: true,
	},

	"SKY SPORTS MX": {
		Logo: "skysports.png",
		Links: []string{
			"http://181.78.106.127:9000/play/ca028/index.m3u8", //
			"http://181.78.106.127:9000/play/ca030/index.m3u8", //
		},
		Name:             "SKY SPORTS MX",
		ShowListChannels: true,
	},

	"SKY SPORTS BUNDESLIGA": {
		Logo: "skysports.png",
		Links: []string{
			// "http://181.78.106.127:9000/play/ca028/32260689.m3u8",
			// "dfbb321c7ee0d3309a03770fea07c67434182acc",
			// "http://181.78.106.127:9000/play/ca028/index.m3u8", //
			// "http://181.205.130.194:4000/play/a07s",
			// "http://200.60.124.19:29000/play/a01e",
			"7c12288663069a12aff58f3f62e8ea47ab78c65d",
			"fc9f1f580da8f641a5991d44b399982a3a069f69",
		},
		Name:             "SKY SPORTS BUNDESLIGA",
		ShowListChannels: true,
	},

	"SKY SPORTS": {
		Logo: "skysports.png",
		Links: []string{
			"http://190.92.10.66:4000/play/a001/index.m3u8",
			"p;https://maldivesn.net/hilaytv/skysportslaliga",
		},
		Name:             "SKY SPORTS LA LIGA",
		ShowListChannels: true,
	},
	"SKY SPORTS PREMIER LEAGUE": {
		Logo: "skysportspremier.png",
		Links: []string{
			"https://d15.epicquesthero.com:1686/hls/skysprem.m3u8?md5=Otw5SigwrXNTvRvU_0kgKg&expires=1758149711", //
		},
		Name:             "SKY SPORTS PREMIER LEAGUE",
		ShowListChannels: true,
	},
	"CANAL + SPORTS": {
		Logo: "canalplusports.png",
		Links: []string{
			"http://116.202.237.33:8080/CNLS3/tracks-v1a1a2a3l1l2/mono.m3u8?token=ef1c6f27da4b04de8c97b52b9255617b", //
		},
		Name:             "CANAL + SPORTS",
		ShowListChannels: true,
	},

	"CANAL + SPORTS 2": {
		Logo: "canalplusports.png",
		Links: []string{
			"http://116.202.237.33:8080/CNLS2/tracks-v1a1a2a3l1l2/mono.m3u8?token=ef1c6f27da4b04de8c97b52b9255617b", //
		},
		Name:             "CANAL + SPORTS 2",
		ShowListChannels: true,
	},

	"REAL MADRID TV": {
		Logo: "realmadridtv.png",
		Links: []string{
			"https://rmtv.akamaized.net/hls/live/2043153/rmtv-es-web/bitrate_3.m3u8", //
		},
		Name:             "REAL MADRID TV",
		ShowListChannels: true,
	},

	"BEIN SPORTS Ñ": {
		Logo: "beinsportn.png",
		Links: []string{
			"https://d35j504z0x2vu2.cloudfront.net/v1/master/0bc8e8376bd8417a1b6761138aa41c26c7309312/bein-sports-xtra-en-espanol/playlist.m3u8", //
		},
		Name:             "BEIN SPORTS Ñ",
		ShowListChannels: true,
	},

	"NBA TV": {
		Logo: "nba.png",
		Links: []string{
			"b0f64a40f333ef5cc02c2b6d4a8c3f4b73dd8073",
			"e72d03fb9694164317260f684470be9ab781ed95",
		},
		Name:             "NBA TV",
		ShowListChannels: true,
	},

	"ESPORTS 3": {
		Logo: "esports3.png",
		Links: []string{
			"ca20f93ea5d9c15744e48a21b52598b9fce87425",
			
		},
		Name:             "ESPORTS 3",
		ShowListChannels: true,
	},

	"Sport TV 1": {
		Logo: "sporttv1.png",
		Links: []string{
			"p;https://maldivesn.net/hilaytv/sporttv1",
			// "http://nfzcdn.royalflushdns.top/live/508373667/k717x9942z/111998.m3u8?sjwt=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE3NTg0OTkxOTksImlhdCI6MTc1ODQxMjgwMCwibmJmIjoxNzU4NDEyODAwLCJ1c2VyIjoiNTA4MzczNjY3IiwidXNlckFnZW50IjoiVkxDLzMuMC4yMSBMaWJWTEMvMy4wLjIxIiwidXNlcklwIjoiMTg1LjIzNi4xODMuMTA3OjU1ODE4In0.j_PDPmv372HwM94IkxMuvxCbyzMgBAvgM9MTgJqHOKI&id=111998&p=m3u8&aid=1758491345",
		},
		Name:             "Sport TV 1",
		ShowListChannels: true,
	},
	"Sport TV 2": {
		Logo: "sporttv2.png",
		Links: []string{
			// "89122d5e60866e9127a5f3849170031b16402adc",
			"p;https://maldivesn.net/hilaytv/sporttv2",
			// "http://nfzcdn.royalflushdns.top/live/508373667/k717x9942z/111998.m3u8?sjwt=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE3NTg0OTkxOTksImlhdCI6MTc1ODQxMjgwMCwibmJmIjoxNzU4NDEyODAwLCJ1c2VyIjoiNTA4MzczNjY3IiwidXNlckFnZW50IjoiVkxDLzMuMC4yMSBMaWJWTEMvMy4wLjIxIiwidXNlcklwIjoiMTg1LjIzNi4xODMuMTA3OjU1ODE4In0.j_PDPmv372HwM94IkxMuvxCbyzMgBAvgM9MTgJqHOKI&id=111998&p=m3u8&aid=1758491345",
		},
		Name:             "Sport TV 2",
		ShowListChannels: true,
	},

	"Sport TV 3": {
		Logo: "sporttv3.png",
		Links: []string{
			// "cf72f0fa0cee438cca43f2ea8299d04d1bf4c9d5",
			"p;https://maldivesn.net/hilaytv/sporttv3",
		},
		Name:             "Sport TV 3",
		ShowListChannels: true,
	},
	"Sport TV 4": {
		Logo: "sporttv4.png",
		Links: []string{
			"p;https://maldivesn.net/hilaytv/sporttv4",
		},
		Name:             "Sport TV 4",
		ShowListChannels: true,
	},
	"Sport TV 5": {
		Logo: "sporttv4.png",
		Links: []string{
			"p;https://maldivesn.net/hilaytv/sporttv5",
		},
		Name:             "Sport TV 5",
		ShowListChannels: true,
	},
	"Sport TV 6": {
		Logo: "sporttv4.png",
		Links: []string{
			"p;https://maldivesn.net/hilaytv/sporttv6",
		},
		Name:             "Sport TV 6",
		ShowListChannels: true,
	},

	// // https://hilay.tv/play.m3u
	"WWE NETWORK": {
		Logo: "wwe.png",
		Links: []string{
			// "http://localhost:3000/wwe/index.m3u8",
			"p;http://fl7.moveonjoy.com/WWE/index.m3u8",
		},
		Name:             "WWE",
		ShowListChannels: true,
	},
	"WWE Superstar Central": {
		Logo: "wwesuper.png",
		Links: []string{
			"p;https://jmp2.uk/stvp-US700005ID",
		},
		Name:             "WWE LEYENDAS",
		ShowListChannels: true,
	},
	"FIFA+": {
		Logo: "fifa.png",
		Links: []string{
			"p;https://jmp2.uk/stvp-ESBC2700009B4",
		},
		Name:             "FIFA+",
		ShowListChannels: true,
	},

	"REDBULL TV": {
		Logo: "redbulltv.png",
		Links: []string{
			"62daab1c54565d0c5ba4e3c660f3a4a5c93adc8a",
			"p;https://jmp2.uk/stvp-GBBD2300003IK",
			"acbf39f533469f3aca35c597dc898d093921291e",
			"acea92d83ba261aa3a72a3c0a662981fa92e0ce9",
		},
		Name:             "REDBULL TV",
		ShowListChannels: true,
	},

	"UFC": {
		Logo: "ufc.png",
		Links: []string{
			"p;https://jmp2.uk/stvp-CA2900012S9",
			"p;https://jmp2.uk/stvp-US2900017P2",
			"p;https://jmp2.uk/plu-677d9adfa9a51b0008497fa0.m3u8",
			// "http://nfzcdn.royalflushdns.top/live/Sndnndd/Dmndndd/731.m3u8?sjwt=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE3NTg1ODU1OTksImlhdCI6MTc1ODQ5OTIwMCwibmJmIjoxNzU4NDk5MjAwLCJ1c2VyIjoiODQxODY2NjU3IiwidXNlckFnZW50IjoiTW96aWxsYS81LjAgKGlQaG9uZTsgQ1BVIGlQaG9uZSBPUyAxNl83XzExIGxpa2UgTWFjIE9TIFgpIEFwcGxlV2ViS2l0LzYwNS4xLjE1IChLSFRNTCwgbGlrZSBHZWNrbykgVmVyc2lvbi8xNi42LjEgTW9iaWxlLzE1RTE0OCBTYWZhcmkvNjA0LjEiLCJ1c2VySXAiOiIxODUuMjM2LjE4My4xMDc6NTU2MjgifQ.mv1pzDQn2-VkoOq3koYh9R7FIIQTc3P99h6ZHvVmrxU&id=731&p=m3u8&aid=1758566817",
		},
		Name:             "UFC",
		ShowListChannels: true,
	},
}

var allCompetitions = AllCompetitions{
	"Sports": CountryCompetitions{
		"Tenis":                  {Titulo: "Tenis", Top: true, Icon: "tenis.png"},
		"FIFA Copa Mundial 2026": {Titulo: "Mundial", Top: true, Icon: "mundial.png"},
		"Mundial Clubes":         {Titulo: "Mundial Clubes", Top: true, Icon: "mundialclubes.png"},
		"FIA Fórmula 2":          {Titulo: "FIA Fórmula 2", Top: true, Icon: "formula2.png"},
		"FIA Fórmula 3":          {Titulo: "FIA Fórmula 3", Top: true, Icon: "formula3.png"},
		"Fórmula 1":              {Titulo: "Fórmula 1", Top: true, Icon: "formula1.png"},
		"Moto2":                  {Titulo: "Moto2", Top: true, Icon: "moto2.png"},
		"Moto3":                  {Titulo: "Moto3", Top: true, Icon: "moto3.png"},
		"MotoGP":                 {Titulo: "MotoGP", Top: true, Icon: "motogp.png"},
		"Boxeo":                  {Titulo: "Boxeo", Top: true, Icon: "boxeo.png"},
		"UFC":                    {Titulo: "UFC", Top: true, Icon: "ufc.png"},
		"WRC Rally":              {Titulo: "WRC Rally", Top: true, Icon: "wrc.png"},
	},
	"España": CountryCompetitions{
		"LaLiga":             {Titulo: "LaLiga", Top: true, Icon: "liga.png"},
		"LaLiga Hypermotion": {Titulo: "LaLiga 2", Top: true, Icon: "liga2.png"},
		"Liga Endesa":        {Titulo: "Liga Endesa", Top: true, Icon: "endesa.png"},
		"Primera FEB":        {Titulo: "Primera FEB", Top: false, Icon: "primerafeb.png"},
		"Primera Federación": {Titulo: "Primera Federación", Top: true, Icon: "primerafede.png"},
		// "Segunda Federación":            {Titulo: "Segunda Federación", Top: false, Icon: "segundafede.png"},
		"Copa del Rey":              {Titulo: "Copa del Rey", Top: true, Icon: "uefa.png"},
		"Supercopa de España":       {Titulo: "Supercopa de España", Top: true, Icon: "uefa.png"},
		"Liga F Moeve":              {Titulo: "Liga F Moeve", Top: false, Icon: "uefa.png"},
		"Copa Federación":           {Titulo: "Copa Federación", Top: false, Icon: "uefa.png"},
		"Supercopa Femenina":        {Titulo: "Supercopa Femenina", Top: false, Icon: "uefa.png"},
		"Copa de SM La Reina":       {Titulo: "Copa de SM La Reina", Top: false, Icon: "uefa.png"},
		"División de Honor Juvenil": {Titulo: "División de Honor Juvenil", Top: false, Icon: "uefa.png"},
		// "Primera Federación Women":      {Titulo: "Primera Federacion Women", Top: false, Icon: "primerafede.png"},
		// "Segunda Federación Femenina":   {Titulo: "Segunda Federación Femenina", Top: false, Icon: "segundafede.png"},
		"Spain U19 Cup":                 {Titulo: "Spain U19 Cup", Top: false, Icon: "uefa.png"},
		"U19 Division de Honor Juvenil": {Titulo: "U19 Division de Honor Juvenil", Top: false, Icon: "uefa.png"},
	},
	"Inglaterra": CountryCompetitions{
		"Premier League":              {Titulo: "Premier League", Top: true, Icon: "premiere.png"},
		"Championship":                {Titulo: "Championship", Top: false, Icon: "uefa.png"},
		"League One":                  {Titulo: "League One", Top: false, Icon: "uefa.png"},
		"League Two":                  {Titulo: "League Two", Top: false, Icon: "uefa.png"},
		"National League":             {Titulo: "National League", Top: false, Icon: "uefa.png"},
		"FA Cup":                      {Titulo: "FA Cup", Top: false, Icon: "uefa.png"},
		"FA Cup, Qualification":       {Titulo: "FA Cup, Qualification", Top: false, Icon: "uefa.png"},
		"EFL Cup":                     {Titulo: "EFL Cup", Top: false, Icon: "uefa.png"},
		"Football League Trophy":      {Titulo: "Football League Trophy", Top: false, Icon: "uefa.png"},
		"FA Women's Championship":     {Titulo: "FA Women's Championship", Top: false, Icon: "uefa.png"},
		"Community Shield":            {Titulo: "Community Shield", Top: false, Icon: "uefa.png"},
		"Women's Super League":        {Titulo: "Women's Super League", Top: false, Icon: "uefa.png"},
		"Women's FA Cup":              {Titulo: "Women's FA Cup", Top: false, Icon: "uefa.png"},
		"FA Women's League Cup":       {Titulo: "FA Women's League Cup", Top: false, Icon: "uefa.png"},
		"England National League Cup": {Titulo: "England National League Cup", Top: false, Icon: "uefa.png"},
		"Baller League UK":            {Titulo: "Baller League UK", Top: false, Icon: "uefa.png"},
		"FA Youth Cup":                {Titulo: "FA Youth Cup", Top: false, Icon: "uefa.png"},
	},
	"Alemania": CountryCompetitions{
		"Bundesliga":    {Titulo: "Bundesliga", Top: true, Icon: "bundesliga.png"},
		"2. Bundesliga": {Titulo: "2. Bundesliga", Top: false, Icon: "uefa.png"},
		"DFB Pokal":     {Titulo: "DFB Pokal", Top: false, Icon: "uefa.png"},
		"DFL Supercup":  {Titulo: "DFL Supercup", Top: false, Icon: "uefa.png"},
	},
	"Italia": CountryCompetitions{
		"Coppa Italia":           {Titulo: "Coppa Italia", Top: true, Icon: "seriea.png"},
		"Serie A Italiana":       {Titulo: "Serie A", Top: true, Icon: "seriea.png"},
		"Serie B":                {Titulo: "Serie B", Top: false, Icon: "uefa.png"},
		"Campionato Primavera 1": {Titulo: "Campionato Primavera 1", Top: false, Icon: "uefa.png"},
		"Campionato Primavera 2": {Titulo: "Campionato Primavera 2", Top: false, Icon: "uefa.png"},
		"Serie C, Playoffs":      {Titulo: "Serie C, Playoffs", Top: false, Icon: "uefa.png"},
		"Supercoppa Serie C":     {Titulo: "Supercoppa Serie C", Top: false, Icon: "uefa.png"},
		"Serie D Poule Scudetto": {Titulo: "Serie D Poule Scudetto", Top: false, Icon: "uefa.png"},
		"Serie A Women":          {Titulo: "Serie A Women", Top: false, Icon: "uefa.png"},
		"Coppa Italia Femminile": {Titulo: "Coppa Italia Femminile", Top: false, Icon: "uefa.png"},
		"Supercoppa Primavera":   {Titulo: "Supercoppa Primavera", Top: false, Icon: "uefa.png"},
		"Trofeo Dossena":         {Titulo: "Trofeo Dossena", Top: false, Icon: "uefa.png"},
		"Serie D, Girone H":      {Titulo: "Serie D, Girone H", Top: false, Icon: "uefa.png"},
		"Serie B Femminile":      {Titulo: "Serie B Femminile", Top: false, Icon: "uefa.png"},
	},
	"Francia": CountryCompetitions{
		"Ligue 1":                  {Titulo: "Ligue 1", Top: true, Icon: "ligue1.png"},
		"Ligue 2":                  {Titulo: "Ligue 2", Top: false, Icon: "uefa.png"},
		"National 1":               {Titulo: "National 1", Top: false, Icon: "uefa.png"},
		"National 2":               {Titulo: "National 2", Top: false, Icon: "uefa.png"},
		"Coupe de France":          {Titulo: "Coupe de France", Top: false, Icon: "uefa.png"},
		"Trophée des Champions":    {Titulo: "Trophée des Champions", Top: false, Icon: "uefa.png"},
		"Première Ligue, Féminine": {Titulo: "Première Ligue, Féminine", Top: false, Icon: "uefa.png"},
		"Coupe de France, Women":   {Titulo: "Coupe de France, Women", Top: false, Icon: "uefa.png"},
		"Championnat National U19": {Titulo: "Championnat National U19", Top: false, Icon: "uefa.png"},
		"Seconde Ligue Women":      {Titulo: "Seconde Ligue Women", Top: false, Icon: "uefa.png"},
	},
	"Europa": CountryCompetitions{
		"Eurobasket":                                {Titulo: "Eurobasket", Top: true, Icon: "eurobasket.png"},
		"Euroliga":                                  {Titulo: "Euroliga", Top: false, Icon: "euroliga.png"},
		"Champions League":                          {Titulo: "UEFA Champions League", Top: true, Icon: "champions.png"},
		"Europa League":                             {Titulo: "UEFA Europa League", Top: true, Icon: "uefa.png"},
		"Conference League":                         {Titulo: "UEFA Conference League", Top: true, Icon: "conference.png"},
		"Super Cup":                                 {Titulo: "UEFA Super Cup", Top: false, Icon: "uefa.png"},
		"Nations League":                            {Titulo: "UEFA Nations League", Top: false, Icon: "uefa.png"},
		"Women's Nations League":                    {Titulo: "UEFA Women's Nations League", Top: false, Icon: "uefa.png"},
		"Women's Euro":                              {Titulo: "Women's Euro", Top: false, Icon: "uefa.png"},
		"Women's Euro, Qualification":               {Titulo: "Women's Euro, Qualification", Top: false, Icon: "uefa.png"},
		"U21 European Championship":                 {Titulo: "U21 European Championship", Top: false, Icon: "uefa.png"},
		"U21 Euro Qualification":                    {Titulo: "U21 Euro Qualification", Top: false, Icon: "uefa.png"},
		"U19 European Championship Qualif.":         {Titulo: "U19 European Championship Qualif.", Top: false, Icon: "uefa.png"},
		"U17 European Championship":                 {Titulo: "U17 European Championship", Top: false, Icon: "uefa.png"},
		"U17 European Championship, Qual.":          {Titulo: "U17 European Championship, Qual.", Top: false, Icon: "uefa.png"},
		"U19 European Women's Championship Qualif.": {Titulo: "U19 European Women's Championship Qualif.", Top: false, Icon: "uefa.png"},
		"U17 European Women's Championship":         {Titulo: "U17 European Women's Championship", Top: false, Icon: "uefa.png"},
		"UEFA Youth League":                         {Titulo: "UEFA Youth League", Top: false, Icon: "uefa.png"},
	},
	"América del Sur": CountryCompetitions{
		"CONMEBOL Libertadores":             {Titulo: "CONMEBOL Libertadores", Top: false, Icon: "uefa.png"},
		"CONMEBOL Sudamericana":             {Titulo: "CONMEBOL Sudamericana", Top: false, Icon: "uefa.png"},
		"CONMEBOL Recopa":                   {Titulo: "CONMEBOL Recopa", Top: false, Icon: "uefa.png"},
		"Copa América":                      {Titulo: "Copa América", Top: false, Icon: "uefa.png"},
		"World Cup Qual. CONMEBOL":          {Titulo: "World Cup Qual. CONMEBOL", Top: false, Icon: "uefa.png"},
		"U17 CONMEBOL Championship":         {Titulo: "U17 CONMEBOL Championship", Top: false, Icon: "uefa.png"},
		"U20 CONMEBOL Libertadores":         {Titulo: "U20 CONMEBOL Libertadores", Top: false, Icon: "uefa.png"},
		"U20 CONMEBOL Championship":         {Titulo: "U20 CONMEBOL Championship", Top: false, Icon: "uefa.png"},
		"U20 CONMEBOL Women's Championship": {Titulo: "U20 CONMEBOL Women's Championship", Top: false, Icon: "uefa.png"},
		"Copa Libertadores Femenina":        {Titulo: "Copa Libertadores Femenina", Top: false, Icon: "uefa.png"},
		"Copa América Femenina":             {Titulo: "Copa América Femenina", Top: false, Icon: "uefa.png"},
		"U17 CONMEBOL Women's Championship": {Titulo: "U17 CONMEBOL Women's Championship", Top: false, Icon: "uefa.png"},
		"U13 Liga Evolución":                {Titulo: "U13 Liga Evolución", Top: false, Icon: "uefa.png"},
		"U16 Liga Evolución, Women":         {Titulo: "U16 Liga Evolución, Women", Top: false, Icon: "uefa.png"},
		"U14 Liga Evolución, Women":         {Titulo: "U14 Liga Evolución, Women", Top: false, Icon: "uefa.png"},
		"U15 CONMEBOL Championship":         {Titulo: "U15 CONMEBOL Championship", Top: false, Icon: "uefa.png"},
		"CONMEBOL Pre-Olympic":              {Titulo: "CONMEBOL Pre-Olympic", Top: false, Icon: "uefa.png"},
	},
	"Brasil": CountryCompetitions{
		"Serie A":          {Titulo: "Serie A", Top: false, Icon: "uefa.png"},
		"Copa do Brasil":   {Titulo: "Copa do Brasil", Top: false, Icon: "uefa.png"},
		"Série B":          {Titulo: "Série B", Top: false, Icon: "uefa.png"},
		"Internacional":    {Titulo: "Internacional", Top: false, Icon: "uefa.png"},
		"Fortaleza SC":     {Titulo: "Fortaleza SC", Top: false, Icon: "uefa.png"},
		"Sport Recife":     {Titulo: "Sport Recife", Top: false, Icon: "uefa.png"},
		"Vasco da Gama":    {Titulo: "Vasco da Gama", Top: false, Icon: "uefa.png"},
		"Grêmio":           {Titulo: "Grêmio", Top: false, Icon: "uefa.png"},
		"Ceará":            {Titulo: "Ceará", Top: false, Icon: "uefa.png"},
		"São Paulo":        {Titulo: "São Paulo", Top: false, Icon: "uefa.png"},
		"Atlético Mineiro": {Titulo: "Atlético Mineiro", Top: false, Icon: "uefa.png"},
		"Palmeiras":        {Titulo: "Palmeiras", Top: false, Icon: "uefa.png"},
	},
	"Argentina": CountryCompetitions{
		"Primera División": {Titulo: "Primera División", Top: false, Icon: "uefa.png"},
		"Copa Argentina":   {Titulo: "Copa Argentina", Top: false, Icon: "uefa.png"},

		// "River Plate":       {Titulo: "River Plate", Top: false, Icon: "uefa.png" },
		// "San Martín SJ":     {Titulo: "San Martín SJ", Top: false, Icon: "uefa.png" },
		// "Racing Avellaneda": {Titulo: "Racing Avellaneda", Top: false, Icon: "uefa.png" },
		// "Unión Santa Fe":    {Titulo: "Unión Santa Fe", Top: false, Icon: "uefa.png" },
		// "Gimnasia LP":       {Titulo: "Gimnasia LP", Top: false, Icon: "uefa.png" },
		// "Atlético Tucumán":  {Titulo: "Atlético Tucumán", Top: false, Icon: "uefa.png" },
		// "Platense":          {Titulo: "Platense", Top: false, Icon: "uefa.png" },
		// "Godoy Cruz":        {Titulo: "Godoy Cruz", Top: false, Icon: "uefa.png" },
		// "Estudiantes LP":    {Titulo: "Estudiantes LP", Top: false, Icon: "uefa.png" },
		// "Aldosivi":          {Titulo: "Aldosivi", Top: false},
		// "Independiente":     {Titulo: "Independiente", Top: false},
		// "Miramar Misiones":  {Titulo: "Miramar Misiones", Top: false},
		// "Cerro Largo":       {Titulo: "Cerro Largo", Top: false},
	},

	"Colombia": CountryCompetitions{
		"Primera A":                {Titulo: "Primera A", Top: false, Icon: "uefa.png"},
		"Copa Colombia":            {Titulo: "Copa Colombia", Top: false, Icon: "uefa.png"},
		"Santa Fe":                 {Titulo: "Santa Fe", Top: false, Icon: "uefa.png"},
		"Once Caldas":              {Titulo: "Once Caldas", Top: false, Icon: "uefa.png"},
		"Deportes Tolima":          {Titulo: "Deportes Tolima", Top: false, Icon: "uefa.png"},
		"Bucaramanga":              {Titulo: "Bucaramanga", Top: false, Icon: "uefa.png"},
		"Águilas Doradas Rionegro": {Titulo: "Águilas Doradas Rionegro", Top: false, Icon: "uefa.png"},
		"Boyacá Chicó":             {Titulo: "Boyacá Chicó", Top: false, Icon: "uefa.png"},
		"LDU Quito":                {Titulo: "LDU Quito", Top: false, Icon: "uefa.png"},
		"El Nacional":              {Titulo: "El Nacional", Top: false, Icon: "uefa.png"},
	},
	"Venezuela": CountryCompetitions{
		"Primera División": {Titulo: "Primera División", Top: false, Icon: "uefa.png"},
		"Trujillanos":      {Titulo: "Trujillanos", Top: false, Icon: "uefa.png"},
		"Héroes de Falcón": {Titulo: "Héroes de Falcón", Top: false, Icon: "uefa.png"},
	},
	"Ecuador": CountryCompetitions{
		"Serie A":      {Titulo: "Serie A", Top: false, Icon: "uefa.png"},
		"Barcelona SC": {Titulo: "Barcelona SC", Top: false, Icon: "uefa.png"},
		"U. Católica":  {Titulo: "U. Católica", Top: false, Icon: "uefa.png"},
		"Delfín SC":    {Titulo: "Delfín SC", Top: false, Icon: "uefa.png"},
		"Libertad FC":  {Titulo: "Libertad FC", Top: false, Icon: "uefa.png"},
		"LDU Quito":    {Titulo: "LDU Quito", Top: false, Icon: "uefa.png"},
		"El Nacional":  {Titulo: "El Nacional", Top: false, Icon: "uefa.png"},
	},
	"Estados Unidos": CountryCompetitions{
		"Major League Soccer (MLS)": {Titulo: "Major League Soccer (MLS)", Top: false, Icon: "uefa.png"},
		"US Open Cup":               {Titulo: "US Open Cup", Top: false, Icon: "uefa.png"},
		"Seattle Sounders":          {Titulo: "Seattle Sounders", Top: false, Icon: "uefa.png"},
		"Inter Miami CF":            {Titulo: "Inter Miami CF", Top: false, Icon: "uefa.png"},
		"Los Angeles FC":            {Titulo: "Los Angeles FC", Top: false, Icon: "uefa.png"},
		"San Diego FC":              {Titulo: "San Diego FC", Top: false, Icon: "uefa.png"},
		"Columbus Crew":             {Titulo: "Columbus Crew", Top: false, Icon: "uefa.png"},
		"New England Revolution":    {Titulo: "New England Revolution", Top: false, Icon: "uefa.png"},
		"FC Cincinnati":             {Titulo: "FC Cincinnati", Top: false, Icon: "uefa.png"},
		"New York City":             {Titulo: "New York City", Top: false, Icon: "uefa.png"},
		"Sporting KC":               {Titulo: "Sporting KC", Top: false, Icon: "uefa.png"},
	},
	"México": CountryCompetitions{
		"Liga MX": {Titulo: "Liga MX", Top: false, Icon: "uefa.png"},
		"Copa MX": {Titulo: "Copa MX", Top: false, Icon: "uefa.png"},

		"Chivas Guadalajara": {Titulo: "Chivas Guadalajara", Top: false, Icon: "uefa.png"},
		"Club América":       {Titulo: "Club América", Top: false, Icon: "uefa.png"},
	},
	"Arabia Saudita": CountryCompetitions{
		"Saudi Professional League": {Titulo: "Saudi Professional League", Top: false, Icon: "uefa.png"},

		"Al Nassr": {Titulo: "Al Nassr", Top: false, Icon: "uefa.png"},
		"Al Hilal": {Titulo: "Al Hilal", Top: false, Icon: "uefa.png"},
	},
}
