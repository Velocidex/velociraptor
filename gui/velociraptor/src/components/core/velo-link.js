import React, { Component } from 'react';

class VeloNavLink extends Component {
    constructor(props) {
        super(props);
    };

    render() {
        return (
             <li grr-nav-link className="nav-link" state="client.flows"
                 disabled={!this.state.client_id}
                 params="{clientId: controller.clientId}" >
               <i className="navicon"><FontAwesomeIcon icon="history"/></i>
               Collected Artifacts
             </li>
        );
    };
};


export default VeloNavLink;
