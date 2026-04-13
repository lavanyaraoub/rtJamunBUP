// Ionic Starter App

// angular.module is a global place for creating, registering and retrieving Angular modules
// 'starter' is the name of this angular module example (also set in a <body> attribute in index.html)
// the 2nd parameter is an array of 'requires'
// 'starter.controllers' is found in controllers.js
//angular.module('starter', ['ionic', 'starter.controllers','ng-virtual-keyboard'])
angular.module('starter', ['ionic', 'starter.controllers','angular-virtual-keyboard'])
.config(['VKI_CONFIG', function(VKI_CONFIG) {
			VKI_CONFIG.layout.Numerico = {
				'name': "Numerico", 'keys': [
				[["1", '1'], ["2", "2"], ["3", "3"], ["Bksp", "Bksp"]],
				[["4", "4"], ["5", "5"], ["6", '6'], ["Enter", "Enter"]],
				[["7", "7"], ["8", "8"], ["9", "9"], []],
				[["0", "0"], ["-"], ["+"], ["."]]
			], 'lang': ["pt-BR-num"]
    };
}])
.directive('ionBottomSheet', [function() {
    return {
      restrict: 'E',
      transclude: true,
      replace: true,
      controller: [function() {}],
      template: '<div class="modal-wrapper" ng-transclude></div>'
    };
  }])
.directive('ionBottomSheetView', function() {
  return {
    restrict: 'E',
    compile: function(element) {
      element.addClass('bottom-sheet modal');
    }
  };
})
.directive('detectGestures', function($ionicGesture) {
  return {
    restrict :  'A',
    link : function(scope, elem, attrs) {
      var gestureType = attrs.gestureType;
      console.log(attrs.gestureType);
      elem.obj_id = attrs.gestureType
      $ionicGesture.on('tap', scope.reportEvent,elem, attrs.gestureType);
      /*switch(gestureType) {
        case 'swipe':
          $ionicGesture.on('swipe', scope.reportEvent, elem, gestureItem);
          break;
        case 'swiperight':
          $ionicGesture.on('swiperight', scope.reportEvent, elem);
          break;
        case 'swipeleft':
          $ionicGesture.on('swipeleft', scope.reportEvent, elem);
          break;
        case 'doubletap':
          $ionicGesture.on('doubletap', scope.reportEvent, elem);
          break;
        case 'tap':
          $ionicGesture.on('tap', scope.reportEvent, elem, gestureItem);
          break;
        case 'scroll':
          $ionicGesture.on('scroll', scope.reportEvent, elem);
          break;
      }*/

    }
  }
})
.run(function($ionicPlatform, $rootScope, $templateCache) {
  $rootScope.$on('$viewContentLoaded', function() {
      $templateCache.removeAll();
   });
  $ionicPlatform.ready(function() {
    // Hide the accessory bar by default (remove this to show the accessory bar above the keyboard
    // for form inputs)
    if (window.cordova && window.cordova.plugins.Keyboard) {
      cordova.plugins.Keyboard.hideKeyboardAccessoryBar(true);
      cordova.plugins.Keyboard.disableScroll(true);

    }
    if (window.StatusBar) {
      // org.apache.cordova.statusbar required
      StatusBar.styleDefault();
    }
  });
})
.config(function($stateProvider, $urlRouterProvider) {
  $stateProvider

  .state('app', {
    url: '/app',
    abstract: true,
    cache: false,
    templateUrl: 'templates/menu.html',
    controller: 'HomeCtrl'
  })
  .state('app.program', {
      url: '/program',
      views: {
        'menuContent': {
          templateUrl: 'templates/program.html',
          controller: 'programCtrl'
        }
      }
  })
  .state('app.settings', {
      url: '/settings',
      views: {
        'menuContent': {
          templateUrl: 'templates/settings.html',
          controller: 'SettingsCtrl'
        }
      }
    })
  .state('app.settings_menu', {
      url: '/settings_menu',
      views: {
        'menuContent': {
          templateUrl: 'templates/settings_menu.html',
          controller: 'SettingsMenuCtrl'
        }
      }
    })
  .state('app.drive_params', {
      url: '/drive_params',
      views: {
        'menuContent': {
          templateUrl: 'templates/drive_params.html',
          controller: 'DriveParamsCtrl'
        }
      }
    })
  .state('app.pitch_error', {
      url: '/pitch_error',
      views: {
        'menuContent': {
          templateUrl: 'templates/pitch_error.html',
          controller: 'PitchErrorCtrl'
        }
      }
    })
    .state('app.drive_offset', {
        url: '/drive_offset',
        views: {
          'menuContent': {
            templateUrl: 'templates/drive_offset.html',
            controller: 'DriveOffsetCtrl'
          }
        }
      })
  .state('app.manual', {
      url: '/manual',
      cache: false,
      views: {
        'menuContent': {
          cache: false,
          templateUrl: 'templates/manual.html',
          controller: 'HomeCtrl'
        }
      }
  })
  .state('app.search', {
    url: '/search',
    views: {
      'menuContent': {
        templateUrl: 'templates/search.html'
      }
    }
  })

  .state('app.browse', {
      url: '/browse',
      views: {
        'menuContent': {
          templateUrl: 'templates/browse.html'
        }
      }
    })
    .state('app.playlists', {
      url: '/playlists',
      views: {
        'menuContent': {
          templateUrl: 'templates/playlists.html',
          controller: 'PlaylistsCtrl'
        }
      }
    })
    .state('app.home', {
      url: '/home',
      views: {
        'menuContent': {
          templateUrl: 'templates/home.html',
          controller: 'HomeCtrl'
        }
      }
    })

    .state('app.auto', {
      url: '/auto',
      params: {file_name: null, contents:null},
      cache: false,
      views: {
        'menuContent': {
          cache: false,
          templateUrl: 'templates/auto.html',
          controller: 'AutoCtrl',

        }
      }
    })
  .state('app.single', {
    url: '/playlists/:playlistId',
    views: {
      'menuContent': {
        templateUrl: 'templates/playlist.html',
        controller: 'PlaylistCtrl'
      }
    }

  });
  // if none of the above states are matched, use this as the fallback
  $urlRouterProvider.otherwise('/app/home');
});

