import React from 'react';
import PropTypes from 'prop-types';

import Tabs from 'react-bootstrap/Tabs';
import Tab from 'react-bootstrap/Tab';

import HuntOverview from './hunt-overview.js';
import HuntRequest from './hunt-request.js';
import HuntClients from './hunt-clients.js';
import HuntNotebook from './hunt-notebook.js';
import Spinner from '../utils/spinner.js';
import T from '../i8n/i8n.js';
import { withRouter }  from "react-router-dom";

class HuntInspector extends React.Component {
    static propTypes = {
        hunt: PropTypes.object,
        fetch_hunts: PropTypes.func,
    };

    state = {
        tab: "overview",
    }

    setDefaultTab = (tab) => {
        this.setState({tab: tab});
        this.props.history.push(
            "/hunts/" + this.props.hunt.hunt_id + "/" + tab);
    }

    render() {
        if (this.props.hunt && this.props.hunt.loading) {
            return <Spinner loading={true}/>;
        }

        if (!this.props.hunt || !this.props.hunt.hunt_id) {
            return <div className="no-content">{T("Please select a hunt above")}</div>;
        }

        // Default tab comes from the router
        let tab = this.props.match && this.props.match.params &&
            this.props.match.params.tab;
        tab = tab || this.state.tab;

        return (
            <div className="padded">
              <Tabs defaultActiveKey={tab} onSelect={this.setDefaultTab}>
                <Tab eventKey="overview" title={T("Overview")}>
                  { tab === "overview" &&
                    <HuntOverview
                      fetch_hunts={this.props.fetch_hunts}
                      hunt={this.props.hunt}/> }
                </Tab>
                <Tab eventKey="requests" title={T("Requests")}>
                  { tab === "requests" &&
                    <HuntRequest hunt={this.props.hunt}/> }
                </Tab>
                <Tab eventKey="clients" title={T("Clients")}>
                  { tab === "clients" &&
                    <HuntClients hunt={this.props.hunt} />}
                </Tab>
                <Tab eventKey="notebook" title={T("Notebook")}>
                  { tab === "notebook" &&
                    <HuntNotebook hunt={this.props.hunt} />}
                </Tab>

              </Tabs>
            </div>
        );
    }
};

export default withRouter(HuntInspector);
