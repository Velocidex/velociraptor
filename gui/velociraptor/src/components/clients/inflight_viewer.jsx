import _ from 'lodash';
import React from 'react';
import PropTypes from 'prop-types';
import VeloTable from "../core/table.jsx";
import { formatColumns } from "../core/table.jsx";
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

        let column_renderers = formatColumns([
            {test: "ClientId", dataField: "ClientId", type: "hidden"},
            {text: "FlowId", dataField: "FlowId", sort: true, type: "flow"},
            {text: "Last Seen Time", dataField: "LastSeenTime", sort: true,
             type: "timestamp"}
        ]);

        let renderers = {
            "ClientId": column_renderers[0],
            "FlowId": column_renderers[1],
            "LastSeenTime": column_renderers[2]};

        return (
            <div>
              <h2>{T("Currently Processing Flows")}</h2>
              <VeloTable rows={rows}
                         columns={columns}
                         no_toolbar={true}
                         column_renderers={renderers}/>
            </div>
        );
    }
};
