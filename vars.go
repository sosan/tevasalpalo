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
			// "b03f9310155cf3b4eafc1dfba763781abc3ff025",
			// "36394be1db2e20b5997d987c32fd38c7f0f194b7",
			// "50a8a13c474848d1efbd5586efdb5b6cdd173fa9",
		},
	},
	"DAZN": {
		Logo:  "dazn1.png",
		Links: []string{
			// "b03f9310155cf3b4eafc1dfba763781abc3ff025",
			// "36394be1db2e20b5997d987c32fd38c7f0f194b7",
			// "50a8a13c474848d1efbd5586efdb5b6cdd173fa9",
			// "688e785893b50acc1d00cb37f15bfc42e72f5ae3",
			// "4141892f5df7d6474bf0279895ce02b7336c9928",
			// "0560234787945a17522e284c4c22bb4df29f33b0",
		},
	},
	"DAZN 2": {
		Logo:  "dazn2.png",
		Links: []string{
			// "43004e955731cd3afcc34d24e5178d4b427bff37",
			// "b0eabe0fdd02fdd165896236765a9b753a2ff516",
		},
	},
	"DAZN 3": {
		Logo:  "dazn3.png",
		Links: []string{},
	},
	"DAZN 4": {
		Logo:  "dazn4.png",
		Links: []string{
			// "4e401fdceebffdf1f09aef954844d09f6c62f460",
			// "eb884f77ce8815cf1028c4d85e8b092c27ea1693",
			// "6a11eb510edc5b3581c5c97883c44563eb894b1b",
			// "7e90956539f4e1318a63f3960e4f75c7c7c5a3b8",
			// "c21a2524a8de3e1e5b126f2677a3e993d9aa07c4",
		},
	},
	"DAZN LALIGA": {
		Logo:  "daznlaliga.png",
		Links: []string{
			// "0e50439e68aa2435b38f0563bb2f2e98f32ff4b1",
			// "1bb5bf76fb2018d6db9aaa29b1467ecdfabe2884",
			// "f8d5e39a49b9da0215bbd3d9efb8fb3d06b76892",
		},
	},
	"DAZN LALIGA 2": {
		Logo:  "daznlaliga.png",
		Links: []string{
			// "5091ea94b75ba4b50b078b4102a3d0e158ef59c3",
			// "c976c7b37964322752db562b4ad65515509c8d36",
		},
	},
	"DSPORT": {
		Logo: "dsport.webp",
		Links: []string{
			"8bdeb6055da0be3bd1e1977dbf3640408f7d0267",
			"http://190.117.20.37:8000/play/a08d/index.m3u8",   //
			"http://179.63.6.17:9000/play/a08e",                //
			"http://181.78.106.127:9000/play/ca026/index.m3u8", //
		},
	},
	"ESPN MX": {
		Logo: "espn.webp",
		Links: []string{
			"http://181.78.106.127:9000/play/ca033/index.m3u8", //"",
		},
	},
	"ESPN MX 2": {
		Logo: "espn.webp",
		Links: []string{
			"http://181.78.106.127:9000/play/ca033/index.m3u8", //"",
		},
	},
	"ESPN 2": {
		Logo: "espn.webp",
		Links: []string{
			"http://181.205.130.194:4000/play/a07i",            //
			"http://181.78.106.127:9000/play/ca034/index.m3u8", //
		},
	},
	"ESPN 3": {
		Logo: "espn.webp",
		Links: []string{
			"http://181.78.106.127:9000/play/ca035/index.m3u8", //
		},
	},
	"ESPN 5": {
		Logo: "espn.webp",
		Links: []string{
			"http://38.41.8.1:8000/play/a07b", //
		},
	},

	"ESPN 6": {
		Logo: "espn.webp",
		Links: []string{
			"http://181.205.130.194:4000/play/a07t", //
			"http://38.44.109.41:8003/play/a00u",    //
			"http://38.41.8.1:8000/play/a082",       //
		},
	},

	"ESPN 7 MX": {
		Logo: "espn.webp",
		Links: []string{
			"http://181.205.130.194:4000/play/a07s", //

		},
	},

	"ESPN": {
		Logo:  "espn.webp",
		Links: []string{
			// "4b048bcfaed7daec454e88f0e29b56756300447d",
		},
	},
	"ESPN DEPORTES": {
		Logo:  "espndeportes.png",
		Links: []string{
			// "4b048bcfaed7daec454e88f0e29b56756300447d",
		},
	},
	"ESPN PREMIUM": {
		Logo: "espnpremium.webp",
		Links: []string{
			"http://190.102.246.93:9005/play/a00x", //
			"http://190.102.246.93:9005/play/a029", //
		},
	},

	"TNT SPORTS": {
		Logo:  "tntsports.png",
		Links: []string{
			// "8bdeb6055da0be3bd1e1977dbf3640408f7d0267",
		},
	},
	"DAZN F1": {
		Logo:  "daznf1.png",
		Links: []string{
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
	},
	"DAZN LALIGA TV": {
		Logo:  "daznlaliga.png",
		Links: []string{
			// "0e50439e68aa2435b38f0563bb2f2e98f32ff4b1",
			// "1bb5bf76fb2018d6db9aaa29b1467ecdfabe2884",
			// "f8d5e39a49b9da0215bbd3d9efb8fb3d06b76892",
			// "520950d296c952e1864a08e15af9f89f1ab514ec",
		},
	},
	"M+ LALIGA": {
		Logo:  "mlaliga.png",
		Links: []string{
			// "107c3ce5a5d2527c9f06e4eab87477201791f231",
			// "d2ddf9802ccfdc456f872eea4d24596880a638a0",
			// "14b6cd8769cd485f2cffdca64be9698d9bfeac58",
			// "07f2b92762cfff99bba785c2f5260c7934ca6034",
			// "4b528d10eaad747ddf52251206177573ee3e9f74",
			// "d3de78aebe544611a2347f54d5796bd87f16c92d",
		},
	},
	"M+ LALIGA TV": {
		Logo:  "mlaligatv.png",
		Links: []string{
			// "14b6cd8769cd485f2cffdca64be9698d9bfeac58",
			// "07f2b92762cfff99bba785c2f5260c7934ca6034",
			// "4b528d10eaad747ddf52251206177573ee3e9f74",
			// "d3de78aebe544611a2347f54d5796bd87f16c92d",
		},
	},
	"M+ LALIGA 2": {
		Logo:  "mlaliga2.png",
		Links: []string{
			// "911ad127726234b97658498a8b790fdd7516541d",
			// "51b363b1c4d42724e05240ad068ad219df8042ec",
			// "ad42faa399df66dcd62a1cbc9d1c99ed4512d3b8",
		},
	},
	"M+ LALIGA 2 TV": {
		Logo:  "mlaliga2.png",
		Links: []string{
			// "911ad127726234b97658498a8b790fdd7516541d",
			// "51b363b1c4d42724e05240ad068ad219df8042ec",
			// "ad42faa399df66dcd62a1cbc9d1c99ed4512d3b8",
		},
	},
	"M+ LALIGA 3": {
		Logo:  "mlaliga3.png",
		Links: []string{
			// "382b14499e3d76e557d449d2e5bbc4e4bd63bd39",
		},
	},
	"M+ LALIGA 3 TV": {
		Logo:  "mlaliga3.png",
		Links: []string{
			// "382b14499e3d76e557d449d2e5bbc4e4bd63bd39",
		},
	},
	"M+ LIGA DE CAMPEONES": {
		Logo:  "mligacampeones.png",
		Links: []string{
			// "0f7842f8b6c26571e5a974407b61623e56e6a052",
			// "f3eea003e23f94dc2d527306de9dd386e3ebf4ba",
			// "680187938f9305cce3ae240298f10e8695bf77c2",
			// "e572a5178ff72eed7d1d751a18b4b3419699f370",
			// "c16b4fab1f724c94cad92081cbb7fc7c6fe8a2cc",
			// "afbf2a479c5a5072698139f0f556ef3e77a99bd0",
			// "dfa66881b9613a77bd5479f6eedc5542504c3ef7",
		},
	},
	"M+ LIGA DE CAMPEONES 2": {
		Logo:  "mligacampeones2.png",
		Links: []string{
			// "e7d8cae7f693fe46e89bbf74820caac9ffb32a30",
			// "33c009a025508cb2186b9ce36279640bb2507bdf",
			// "74ab4e4ec7e2da001f473ca40893b7307b8029c5",
			// "4fc6d0331830ad8743abab2fe2473b63bdfbc49f",
		},
	},
	"M+ LIGA DE CAMPEONES 3": {
		Logo:  "mligacampeones3.png",
		Links: []string{
			// "2b5129adc57d43790634d796fe3468b9fd061259",
			// "17b8bc4bf8e29547b0071c742e3d7da3bcbc484d",
			// "4416843c96b7f7a1bc55c476091a60fff0922bc7",
		},
	},
	"M+ LIGA DE CAMPEONES 4": {
		Logo:  "mligacampeones4.png",
		Links: []string{
			// "77998f8161373611ff6b348e7eda5b4e97d3ec29",
		},
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
	},
	"M+ DEPORTES 3": {
		Logo:  "mdeportes3.png",
		Links: []string{
			// "29d786d72d4b53dbc333af3a50f8fb021aa0296f",
			// "d5271a967767f761c8812c4b6195dd40f20126f7",
			// "753d4b1f7c4ef43238b5cf23b05572b550a44eee",
		},
	},
	"M+ DEPORTES 4": {
		Logo:  "mdeportes4.png",
		Links: []string{
			// "37825883ed185365e3194a79207347f6c7bd5ba5",
			// "ebf3f251c1e119aefc6a1efc95c9b5d1909249e2",
			// "58a4c880ab18486d914751db32a12760e74b75a4",
		},
	},
	"M+ DEPORTES 5": {
		Logo:  "mdeportes5.png",
		Links: []string{
			// "6dc4ac4eeae18e9daec433b01db82435cf84c57c",
			// "9b84af74b2fa3690c519199326fc2f181b025cdd",
			// "0b708083541a46dc15216c8003d7bcf39c458b2a",
		},
	},
	"M+ DEPORTES 6": {
		Logo:  "mdeportes6.png",
		Links: []string{
			// "190a81938c2ddc6fe97758271f8c48f4db31c28a",
		},
	},

	"M+ VAMOS": {
		Logo:  "mvamos.png",
		Links: []string{
			// "4e99e755aa32c4bc043a4bb1cd1de35f9bd94dde",
			// "1444a976d2cf6e7fdcee833ed36ee5e55632253f",
			// "c7c81acdd1a03ecc418c94c2f28e2adb0556c40b",
			// "3b2a8b41e7097c16b0468b42d7de0320886fa933",
			// "2940120bf034db79a7f5849846ccea0255172eae",
		},
	},

	"M+ GOLF": {
		Logo:  "mgolf.png",
		Links: []string{
			// "76a69812c66bfc4899e89df498220588a56e6064",
			// "872608e734992db636eb79426802cd08f4029afb",
		},
	},
	"Movistar Golf": {
		Logo:  "mgolf.png",
		Links: []string{
			// "76a69812c66bfc4899e89df498220588a56e6064",
			// "872608e734992db636eb79426802cd08f4029afb",
		},
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
	},
	"Movistar Plus+": {
		Logo:  "m.png",
		Links: []string{
			// "199190e3f28c1de0be34bccf0d3568dc65209b99",
			// "5866e83279307bf919068ae7a0d250e4e424e464",
			// "5d9a26e0a5f3e5f2ae027bd05635ab9a4fd4b51a",
			// "5e24a1b9187fccb91553f7c7da4b36341386f74a",
		},
	},
	"Movistar Plus+ 2": {
		Logo:  "m2.png",
		Links: []string{},
	},
	"Movistar Plus": {
		Logo:  "m.png",
		Links: []string{
			// "199190e3f28c1de0be34bccf0d3568dc65209b99",
			// "5866e83279307bf919068ae7a0d250e4e424e464",
			// "5d9a26e0a5f3e5f2ae027bd05635ab9a4fd4b51a",
			// "5e24a1b9187fccb91553f7c7da4b36341386f74a",
		},
	},
	"LALIGA HYPERMOTION": {
		Logo:  "mlaligahyper.png",
		Links: []string{
			// "8ee52f6208e33706171856f99d2ed2dabd317f3a",
			// "70f22be1286ef224b5e4e9451d9a42468152cda4",
			// "f15f997f457e49ad9697e65cf2d78db26ee875b9",
			// "ff38b875b60074d60edb64cf10d09b32370a7135",
			// "778d2f60bb7207addedcca0b9aed98f41529724e",
		},
	},
	"LALIGA HYPERMOTION 2": {
		Logo:  "mlaligahyper.png",
		Links: []string{
			// "8a05571c0c8fe53f160fb7d116cdf97243679e86",
		},
	},
	"LALIGA HYPERMOTION 3": {
		Logo:  "mlaligahyper.png",
		Links: []string{
			// "1ba18731a8e18bb4b3a5dfa5bb7b0f05762849a6",
		},
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
	},

	"GOL": {
		Logo:  "gol.png",
		Links: []string{
			// "b2d560741c006fc5e4a42412bb52dbd25a6a4a3a",
			// "472d1f3a658a51bcab0ffa9138e1e28a05fba30b",
			// "b2d560741c006fc5e4a42412bb52dbd25a6a4a3a",
		},
	},
	"GOL TV": {
		Logo:  "goltv.png",
		Links: []string{
			// "b2d560741c006fc5e4a42412bb52dbd25a6a4a3a",
			// "472d1f3a658a51bcab0ffa9138e1e28a05fba30b",
			// "b2d560741c006fc5e4a42412bb52dbd25a6a4a3a",
		},
	},
	"GOLTV PLAY": {
		Logo:  "golt.png",
		Links: []string{
			// "b2d560741c006fc5e4a42412bb52dbd25a6a4a3a",
			// "472d1f3a658a51bcab0ffa9138e1e28a05fba30b",
			// "b2d560741c006fc5e4a42412bb52dbd25a6a4a3a",
		},
	},
	"EUROSPORT 1": {
		Logo:  "eurosport1.png",
		Links: []string{
			// "ef2abf419586d9876370be127ad592dbb41b126a",
			// "bae98f69fbf867550b4f73b4eb176dae84d7f909",
			// "714e14e6d6e27660fd25a75b57b0ac5580fe705d",
		},
	},
	"EUROSPORT 2": {
		Logo:  "eurosport2.png",
		Links: []string{
			// "37d6f1aabcde81ee6e4873b4db6b7bb8964af8bf",
			// "dc4ccb9e72550bc72be9360aa7d77e337ad11ecc",
			// "0585e09bb8ac9720e4c11934f1b184e309291551",
			// "5c910d614894635153a7d42de98cc2e4a958a53f",
		},
	},
	"M+ ELLAS VAMOS": {
		Logo:  "mellasvamos.png",
		Links: []string{
			// "9b84af74b2fa3690c519199326fc2f181b025cdd",
		},
	},
	"LALIGA TV BAR": {
		Logo:  "laligatvbar.png",
		Links: []string{
			// "608b0faf7d3d25f6fe5dba13d5e4b4142949990e",
		},
	},
	"TUDN": {
		Logo: "tudn.png",
		Links: []string{
			"http://181.78.106.127:9000/play/ca036/index.m3u8", //
		},
	},

	"FOX SPORTS": {
		Logo: "foxsports1.png",
		Links: []string{
			"http://181.78.106.127:9000/play/ca031/index.m3u8", //
			"http://200.115.120.1:8000/play/ca041/index.m3u8",  //
		},
	},
	"FOX NFL": {
		Logo: "foxsports1.png",
		Links: []string{
			"http://200.115.120.1:8000/play/ca041/index.m3u8", //
		},
	},

	"FOX SPORTS 2": {
		Logo: "foxsports1.png",
		Links: []string{
			"http://181.78.106.127:9000/play/ca032/index.m3u8", //
		},
	},

	"SKY SPORTS MX": {
		Logo: "skysports.png",
		Links: []string{
			"http://181.78.106.127:9000/play/ca028/index.m3u8", //
			"http://181.78.106.127:9000/play/ca030/index.m3u8", //
		},
	},

	"SKY SPORTS LA LIGA": {
		Logo: "skysports.png",
		Links: []string{
			"http://190.92.10.66:4000/play/a001/index.m3u8", //

		},
	},

	"CANAL + SPORTS": {
		Logo: "canalplusports.png",
		Links: []string{
			"http://116.202.237.33:8080/CNLS3/tracks-v1a1a2a3l1l2/mono.m3u8?token=ef1c6f27da4b04de8c97b52b9255617b", //
		},
	},

	"CANAL + SPORTS 2": {
		Logo: "canalplusports.png",
		Links: []string{
			"http://116.202.237.33:8080/CNLS2/tracks-v1a1a2a3l1l2/mono.m3u8?token=ef1c6f27da4b04de8c97b52b9255617b", //
		},
	},

	"REAL MADRID TV": {
		Logo: "realmadridtv.png",
		Links: []string{
			"https://rmtv.akamaized.net/hls/live/2043153/rmtv-es-web/bitrate_3.m3u8", //
		},
	},

	"BEIN SPORTS Ñ": {
		Logo: "beinsportn.png",
		Links: []string{
			"https://d35j504z0x2vu2.cloudfront.net/v1/master/0bc8e8376bd8417a1b6761138aa41c26c7309312/bein-sports-xtra-en-espanol/playlist.m3u8", //
		},
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
		// "Primera Federacion":            {Titulo: "Primera Federacion", Top: false, Icon: "primerafede.png"},
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
		"Serie A":                {Titulo: "Serie A", Top: true, Icon: "seriea.png"},
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
		"UEFA Champions League":                     {Titulo: "UEFA Champions League", Top: true, Icon: "champions.png"},
		"UEFA Europa League":                        {Titulo: "UEFA Europa League", Top: true, Icon: "uefa.png"},
		"UEFA Conference League":                    {Titulo: "UEFA Conference League", Top: true, Icon: "conference.png"},
		"UEFA Super Cup":                            {Titulo: "UEFA Super Cup", Top: false, Icon: "uefa.png"},
		"UEFA Nations League":                       {Titulo: "UEFA Nations League", Top: false, Icon: "uefa.png"},
		"UEFA Women's Nations League":               {Titulo: "UEFA Women's Nations League", Top: false, Icon: "uefa.png"},
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
