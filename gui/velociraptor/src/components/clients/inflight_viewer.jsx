import _ from 'lodash';
import React from 'react';
import PropTypes from 'prop-types';
import VeloTable,  { getFormatter } from "../core/table.jsx";
import T from '../i8n/i8n.jsx';


export default class InFlightViewer extends React.Component {
    static propTypes = {
        client_info: PropTypes.object,
    };

    render() {
        let rows = _.map(this.props.client_info.in_flight_flows, (v, k)=>{
            return {ClientId: this.props.client_info.client_id,
                    FlowId: k, LastSeenTime: v};
        });

        let columns = ["FlowId", "LastSeenTime"];
        let header = {
            LastSeenTime: T("Last Seen Time"),
        };

        let renderers = {
            "ClientId": getFormatter("hidden"),
            "FlowId": getFormatter("flow"),
            "LastSeenTime": getFormatter("timestamp"),
        };

        return (
            <div>
              <h2>{T("Currently Processing Flows")}</h2>
              <VeloTable rows={rows}
                         columns={columns}
                         no_toolbar={true}
                         header_renderers={header}
                         column_renderers={renderers}/>
            </div>
        );
    }
};
