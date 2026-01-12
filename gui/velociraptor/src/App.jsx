import React, { Component } from 'react';
import './css/App.css';
import qs from "qs";
import _ from 'lodash';

import PropTypes from 'prop-types';
import VeloNavigator from './components/sidebar/navigator.jsx';
import GlobalMessages from './components/users/global-messages.jsx';
import VeloClientSearch from './components/clients/search.jsx';
import VeloClientList from './components/clients/clients-list.jsx';
import DocButton from './components/docs/doc-button.jsx';
import VeloHostInfo from './components/clients/host-info.jsx';
import ServerInfo from './components/server/server-info.jsx';
import ClientSetterFromRoute from './components/clients/client_info.jsx';
import VeloClientSummary from './components/clients/client-summary.jsx';
import VFSViewer from './components/vfs/browse-vfs.jsx';
import VeloLiveClock from './components/utils/clock.jsx';
import ClientFlowsView from './components/flows/client-flows-view.jsx';
import ServerFlowsView from './components/flows/server-flows-view.jsx';
import Notebook from './components/notebooks/notebook.jsx';
import FullScreenTable from './components/notebooks/table_view.jsx';
import FullScreenNotebook from './components/notebooks/full_notebook.jsx';
import FullScreenHuntNotebook from './components/hunts/hunt-full-notebook.jsx';
import FullScreenFlowNotebook from './components/flows/flow-full-notebook.jsx';
import ArtifactInspector from './components/artifacts/artifacts.jsx';
import UserInspector from './components/users/user-inspector.jsx';
import VeloHunts from './components/hunts/hunts.jsx';
import UserDashboard from './components/sidebar/user-dashboard.jsx';
import UserLabel from './components/users/user-label.jsx';
import EventMonitoring from './components/events/events.jsx';
import SnackbarProvider from 'react-simple-snackbar';
import Snackbar from './components/core/snackbar.jsx';
import Welcome from './components/welcome/welcome.jsx';
import LoginPage from './components/welcome/login.jsx';
import LogoffPage from './components/welcome/logoff.jsx';
import KeyboardHelp from './components/core/keyboard-help.jsx';
import { UserSettings } from './components/core/user.jsx';
import { Switch, Route, withRouter } from "react-router-dom";
import { Join } from './components/utils/paths.jsx';
import SecretManager from './components/secrets/secrets.jsx';
import ButtonGroup from 'react-bootstrap/ButtonGroup';

import Navbar from 'react-bootstrap/Navbar';
import Nav from 'react-bootstrap/Nav';

import SidebarKeyNavigator from './components/sidebar/hotkeys.jsx';

import './themes/no-theme.css';
import './themes/veloci-light.css';
import './themes/veloci-docs.css';
import './themes/veloci-dark.css';
import './themes/pink-light.css';
import './themes/github-dimmed-dark.css';
import './themes/ncurses-light.css';
import './themes/ncurses-dark.css';
import './themes/coolgray-dark.css';
import './themes/midnight.css';
import './themes/vscode-dark.css';

/* This is the main App page.

  We maintain internal state:

  client - the currently selected client the user will interact
     with. We hold the entire client info object in the top level
     state.

  query - the current search query typed into the top toolbar - this
     is always exposed and only changed by the user. It allows us to
     re-search for the same term quickly.

  The client information is populated via the setClient() callback by:
  1. VeloClientList component receiving a click on a table row.

*/

class App extends Component {
    static propTypes = {
        history: PropTypes.any,
        location: PropTypes.any,
    }

    state = {
        client: {},

        // The file listing in the current directory.
        current_node: {},

        // The current row within the current node that is selected.
        selected_row: {},

        query: "",
        query_version: "",
    }

    // Called to update the current client.
    setClient = (client) => {
        this.setState({client: client});
    };

    setClientSearch = (query) => {
        if (!_.isString(query)) {
            return;
        }
        let now = new Date();
        this.setState({query: query, query_version: now.getTime()});
        this.props.history.push('/search/' + (query || "all"));
    };

    updateCurrentNode = (node) => {
        this.setState({current_node: node});
    }

    updateOrgIdFromUrl = ()=>{
        let search = this.props.location.search.replace('?', '');
        let params = qs.parse(search);
        let org_id = params.org_id;
        if (org_id) {
            window.globals.OrgId = org_id;
        }
    }

    // Renders the entire app as normal.
    renderApp() {
        this.updateOrgIdFromUrl();

        // We need to prepare a vfs_path for the navigator to link
        // to. Depending on the current node, we make a link with or
        // without a final "/".
        let vfs_path = "/";
        if (this.state.current_node && this.state.current_node.path) {
            let path = [...this.state.current_node.path];
            let name = this.state.current_node.selected && this.state.current_node.selected.Name;
            if (name) {
                path.push(name);
                vfs_path = Join(path);
            } else {
                vfs_path = Join(path) + "/";
            }
        }

        return <div>
                 <Navbar fixed="top" className="main-navbar justify-content-between">
                   <div className="navigator-left-side form-inline">
                     <VeloNavigator
                       vfs_path={vfs_path}
                       client={this.state.client} />
                     <VeloClientSearch
                       setSearch={this.setClientSearch} />
                   </div>
                   <VeloClientSummary
                     setClient={this.setClient}
                     client={this.state.client}/>
                   <UserLabel
                     setClient={this.setClient}
                     className="navbar-text"/>
                 </Navbar>
                 <div id="content">
                   <Switch>
                     <Route path="/dashboard">
                       <UserDashboard />
                     </Route>
                     <Route exact path="/" component={Welcome}/>
                     <Route exact path="/welcome" component={Welcome}/>
                     <Route path="/search/:query?">
                       <VeloClientList
                         setSearch={this.setClientSearch}
                         version={this.state.query_version}
                         query={this.state.query}
                         setClient={this.setClient}
                       />
                     </Route>
                     <Route path="/secrets/">
                       <SecretManager/>
                     </Route>
                     <Route path="/artifacts/:artifact?/:action?">
                       <ArtifactInspector client={this.state.client}/>
                     </Route>
                     <Route path="/users/:user?" component={UserInspector}/>
                     <Route path="/hunts/:hunt_id?/:tab?">
                       <VeloHunts/>
                     </Route>
                     <Route path="/host/:client_id([^/]{7,})/:action?">
                       <ClientSetterFromRoute
                         client={this.state.client}
                         setClient={this.setClient} />
                       <VeloHostInfo
                         client={this.state.client}
                         setClient={this.setClient}
                       />
                     </Route>
                     <Route path="/host/:client_id(server)/:action?">
                       <ServerInfo  />
                     </Route>
                     <Route path="/vfs/:client_id/:vfs_path(.*)">
                       <ClientSetterFromRoute client={this.state.client}
                                              setClient={this.setClient} />
                       <VFSViewer client={this.state.client}
                                  selectedRow={this.state.selected_row}
                                  updateCurrentNode={this.updateCurrentNode}
                                  node={this.state.current_node}
                                  vfs_path={this.state.vfs_path} />
                     </Route>
                     {/* ClientFlowsView will only be invoked when the
                       * client looks like a client id - the
                       * ServerFlowsView is invoked when client_id ==
                       * "server". For now we assume the client_id is
                       * longer than 7 chars
                       */}
                     <Route path="/collected/:client_id([^/]{7,})/:flow_id?/:tab?">
                       <ClientSetterFromRoute client={this.state.client} setClient={this.setClient} />
                       <ClientFlowsView client={this.state.client} />
                     </Route>
                     <Route path="/collected/server/:flow_id?/:tab?">
                       <ServerFlowsView />
                     </Route>
                     <Route path="/notebooks/:notebook_id?">
                       <Notebook />
                     </Route>
                     <Route path="/events/:client_id([^/]{7,})/:artifact?/:time?">
                       <ClientSetterFromRoute client={this.state.client} setClient={this.setClient} />
                       <EventMonitoring client={this.state.client}/>
                     </Route>
                     <Route path="/events/server/:artifact?/:time?">
                       <EventMonitoring client={{client_id: ""}}/>
                     </Route>
                   </Switch>
                 </div>
                 <Navbar fixed="bottom" className="app-footer justify-content-between ">
                   <Nav>
                     <ButtonGroup>
                       <GlobalMessages/>
                       <DocButton />
                     </ButtonGroup>
                   </Nav>
                   <Nav>
                     <VeloLiveClock className="float-right" />
                     <Snackbar />
                   </Nav>
                 </Navbar>
               </div>;
    }

    renderMainPage() {
        return (
            <div>
              <UserSettings>
                <SnackbarProvider>
                  <SidebarKeyNavigator client={this.state.client}/>
                  <Switch>
                    <Route path="/fullscreen/notebooks/:notebook_id">
                      <FullScreenNotebook />
                    </Route>
                    <Route path="/fullscreen/hunts/:hunt_id/notebook">
                      <FullScreenHuntNotebook />
                    </Route>
                    <Route path="/fullscreen/collected/:client_id/:flow_id/notebook">
                      <FullScreenFlowNotebook />
                    </Route>
                    <Route path="/fullscreen/table/:state">
                      <FullScreenTable />
                    </Route>
                    <Route>
                      { this.renderApp() }
                    </Route>
                  </Switch>
                  <KeyboardHelp />
                </SnackbarProvider>
              </UserSettings>
            </div>
        );
    };

    render() {
        // This is an error injected into the main page by the server
        // template renderer. We pick it up from here and override
        // rendering the main page with this special login page.
        if (window.ErrorState) {
            if (window.ErrorState.Type === "Login") {
                return <LoginPage/>;
            }
            if (window.ErrorState.Type === "Logoff") {
                return <LogoffPage/>;
            }
        }

        return this.renderMainPage();
    };
}

export default withRouter(App);
