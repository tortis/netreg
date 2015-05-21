var netregApp = angular.module("NetregApp", ['ngRoute']);

netregApp.config(function($routeProvider, $locationProvider) {
	$routeProvider
		.when('/', {
			templateUrl: 'login.html',
			controller: 'LoginCtrl'
		})
		.when('/home', {
			templateUrl: 'home.html',
			controller: 'HomeCtrl'
		});
});

netregApp.controller("LoginCtrl", function($scope, $http, $location, $window) {
	$scope.login = function(user) {
		$http({
			method: 'POST',
			url: '/login',
			data: user,
			headers: {'Content-Type': 'application/x-www-form-urlencoded'},
			transformRequest: function(obj) {
				var str = [];
				for (var p in obj)
					str.push(encodeURIComponent(p) + "=" + encodeURIComponent(obj[p]));
				return str.join("&");
			}
		}).success(function(data) {
			$window.localStorage['token'] = data;
			$scope.loginErr = null;
			console.log("Authentication successful");
			$location.path("/home");
		}).error(function(data, status) {
			if (status >= 400 && status < 500) {
				$scope.loginErr = "Incorrect username or password.";
			} else {
				$scope.loginErr = "Oops! Server error, please try again.";
			}
		});
	};	
});

netregApp.controller("HomeCtrl", function($scope, $http, $window, $location) {
	// Token management
	$scope.getToken = function() {
		var token = $window.localStorage['token'];
		if (!token) {
			return null;
		}
		var tokenBody = token.split('.')[1];
		var tokenJson = atob(tokenBody);
		return JSON.parse(tokenJson);
	};
	$scope.token = $scope.getToken();
	if (!$scope.token) {
		$location.path("/");
		return;
	}
	if ($scope.token.exp < (Math.floor(Date.now() / 1000))) {
		$location.path("/");
		return;
	}
	$scope.username = $scope.token.contents.username;
	if ($scope.token.contents.admin == "yes") {
		$scope.isAdmin = true;
	}

	// Load data
	$scope.load = function() {
		$scope.message = $scope.error = null;
		$http({
			method: 'GET',
			url: '/devices',
			headers: {'Authorization': $window.localStorage['token']}
		}).success(function(data) {
			$scope.devices = data;
		}).error(function(data, status) {
			$scope.error = data;
			console.log(data);
		});
	};
	$scope.load();


	// Functions
	$scope.signout = function() {
		$window.localStorage['token'] = null;
		$location.path("/");
	};

	$scope.startEditing = function(dev) {
		dev.updated = {};
		dev.updated.MAC = dev.MAC;
		dev.updated.Device = dev.Device;
		dev.updated.Enabled = dev.Enabled;
		dev.updated.Owner = dev.Owner;
		dev.editing = true;
	};

	$scope.startAdding = function() {
		$scope.devices.newDev = null;
		$scope.devices.adding = true;
	};

	$scope.toggleEnable = function(dev) {
		dev.updated = {};
		dev.updated.MAC = dev.MAC;
		dev.updated.Device = dev.Device;
		dev.updated.Enabled = !dev.Enabled;
		$scope.updateDevice(dev);
	}

	$scope.updateDevice = function(dev) {
		console.log("Updating device: "+dev.Name);
		$http({
			method: 'PUT',
			url: '/devices/' + dev.MAC,
			data: dev.updated,
			headers: {'Authorization': $window.localStorage['token']}
		}).success(function(data) {
			$scope.load();
			$scope.message = "Successfully updated device.";
		}).error(function(data, status) {
			if (status >= 400 && status < 500) {
				$scope.error = data;
			} else {
				$scope.error = "Oops! Server error, please try again.";
			}
		});
	};

	$scope.addDevice = function() {
		$http({
			method: 'POST',
			url: '/devices',
			data: $scope.devices.newDev,
			headers: {'Authorization': $window.localStorage['token']}
		}).success(function(data) {
			$scope.load();
			$scope.message = "Successfully added " + data.Device;
		}).error(function(data, status) {
			if (status >= 400 && status < 500) {
				$scope.error = data;
			} else {
				$scope.error = "Oops! Server error, please try again.";
			}
		});
	};

	$scope.deleteDevice = function(dev) {
		$http({
			method: 'DELETE',
			url: '/devices/'+dev.MAC,
			headers: {'Authorization': $window.localStorage['token']}
		}).success(function(data) {
			$scope.load();
			$scope.message = "Successfully deleted " + dev.Device;
		}).error(function(data, status) {
			if (status >= 400 && status < 500) {
				$scope.error = data;
			} else {
				$scope.error = "Oops! Server error, please try again.";
			}
		});
	}
});
