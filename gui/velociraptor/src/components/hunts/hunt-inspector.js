import React from 'react';
import PropTypes from 'prop-types';

import Tabs from 'react-bootstrap/Tabs';
import Tab from 'react-bootstrap/Tab';

import HuntOverview from './hunt-overview.js';
import HuntRequest from './hunt-request.js';
import HuntResults from './hunt-results.js';
import HuntClients from './hunt-clients.js';
import HuntStatus from './hunt-status.js';

import { withRouter }  from "react-router-dom";

class HuntInspector extends React.Component {
    static propTypes = {
        hunt: PropTypes.object,
    };

    setDefaultTab = (tab) => {
        this.props.history.push(
            "/hunts/" + this.props.hunt.hunt_id + "/" + tab);
    }

    render() {
        if (!this.props.hunt || !this.props.hunt.hunt_id) {
            return <div>Please select a hunt above</div>;
        }

        // Default tab comes from the router
        let default_tab = this.props.match && this.props.match.params &&
            this.props.match.params.tab;
        default_tab = default_tab || "overview";

        return (
            <div className="padded">
              <Tabs defaultActiveKey={default_tab} onSelect={this.setDefaultTab}>
                <Tab eventKey="overview" title="Overview">
                  <HuntOverview hunt={this.props.hunt}/>
                </Tab>
                <Tab eventKey="requests" title="Requests">
                  <HuntRequest hunt={this.props.hunt}/>
                </Tab>
                <Tab eventKey="results" title="Results">
                  <HuntResults hunt={this.props.hunt} />
                </Tab>
                <Tab eventKey="clients" title="Clients">
                  <HuntClients hunt={this.props.hunt} />
                </Tab>
                <Tab eventKey="status" title="Status">
                  <HuntStatus hunt={this.props.hunt} />
                </Tab>
              </Tabs>
            </div>
        );
    }
};

export default withRouter(HuntInspector);
